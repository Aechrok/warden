package store_test

import (
	"context"
	"testing"

	"github.com/aechrok/warden/internal/store"
	"github.com/aechrok/warden/internal/testutil"
)

func TestOutboxStore_ClaimAndAck(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	ob := store.NewOutboxStore()

	// Enqueue two messages in separate transactions.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	msg1, err := ob.Enqueue(ctx, tx1, "test.topic", map[string]any{"seq": 1})
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Enqueue msg1: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	msg2, err := ob.Enqueue(ctx, tx2, "test.topic", map[string]any{"seq": 2})
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("Enqueue msg2: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit tx2: %v", err)
	}
	_ = msg2

	// Claim one message.
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	claimed, err := ob.ClaimBatch(ctx, tx3, 1)
	if err != nil {
		_ = tx3.Rollback(ctx)
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(claimed) != 1 {
		_ = tx3.Rollback(ctx)
		t.Fatalf("expected 1 claimed message, got %d", len(claimed))
	}
	if claimed[0].ID != msg1.ID {
		_ = tx3.Rollback(ctx)
		t.Errorf("expected msg1 to be claimed first (FIFO), got %v", claimed[0].ID)
	}

	// Ack the claimed message.
	if err := ob.Ack(ctx, tx3, claimed[0].ID); err != nil {
		_ = tx3.Rollback(ctx)
		t.Fatalf("Ack: %v", err)
	}
	if err := tx3.Commit(ctx); err != nil {
		t.Fatalf("commit tx3: %v", err)
	}

	// Claim again — should get msg2.
	tx4, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx4: %v", err)
	}
	defer func() { _ = tx4.Rollback(ctx) }()

	claimed2, err := ob.ClaimBatch(ctx, tx4, 1)
	if err != nil {
		t.Fatalf("ClaimBatch second time: %v", err)
	}
	if len(claimed2) != 1 {
		t.Fatalf("expected 1 message on second claim, got %d", len(claimed2))
	}
	if claimed2[0].ID != msg2.ID {
		t.Errorf("expected msg2 on second claim, got %v", claimed2[0].ID)
	}
}

func TestOutboxStore_Nack(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	ob := store.NewOutboxStore()

	// Enqueue a message.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	msg, err := ob.Enqueue(ctx, tx1, "test.nack", map[string]any{"x": 1})
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Enqueue: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit enqueue: %v", err)
	}

	// Claim it.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	claimed, err := ob.ClaimBatch(ctx, tx2, 1)
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(claimed) != 1 {
		_ = tx2.Rollback(ctx)
		t.Fatalf("expected 1 claimed, got %d", len(claimed))
	}

	// Nack it.
	if err := ob.Nack(ctx, tx2, msg.ID, "transient error"); err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("Nack: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit nack: %v", err)
	}

	// The message should be reclaimable (status=pending).
	tx3, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer func() { _ = tx3.Rollback(ctx) }()

	reclaimable, err := ob.ClaimBatch(ctx, tx3, 1)
	if err != nil {
		t.Fatalf("ClaimBatch after nack: %v", err)
	}
	if len(reclaimable) != 1 {
		t.Fatalf("expected message reclaimable after nack, got %d", len(reclaimable))
	}
	if reclaimable[0].ID != msg.ID {
		t.Errorf("expected same message to be reclaimable, got %v", reclaimable[0].ID)
	}
	// Attempts should be incremented (was 1 after first claim, now 2).
	if reclaimable[0].Attempts < 2 {
		t.Errorf("expected attempts >= 2 after nack+reclaim, got %d", reclaimable[0].Attempts)
	}
}
