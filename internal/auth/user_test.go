package auth_test

import (
	"context"
	"testing"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/testutil"
)

func TestUserStore_GetOrCreate_upsert(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	us := auth.NewUserStore()

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	user1, err := us.GetOrCreate(ctx, tx1, "upsert@example.com", "Original Name")
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("GetOrCreate initial: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit initial: %v", err)
	}

	if user1.Name != "Original Name" {
		t.Errorf("initial name = %q, want %q", user1.Name, "Original Name")
	}

	// Call again with a different name — should update the name.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	user2, err := us.GetOrCreate(ctx, tx2, "upsert@example.com", "Updated Name")
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("GetOrCreate upsert: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit upsert: %v", err)
	}

	if user2.ID != user1.ID {
		t.Errorf("user IDs should match after upsert: got %v, want %v", user2.ID, user1.ID)
	}
	if user2.Name != "Updated Name" {
		t.Errorf("name after upsert = %q, want %q", user2.Name, "Updated Name")
	}
}

func TestUserStore_SetActive_false(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	us := auth.NewUserStore()

	// Create user.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	user, err := us.GetOrCreate(ctx, tx, "deactivate@example.com", "To Deactivate")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("GetOrCreate: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if !user.IsActive {
		t.Fatal("user should be active initially")
	}

	// Deactivate.
	if err := us.SetActive(ctx, pool, user.ID, false); err != nil {
		t.Fatalf("SetActive(false): %v", err)
	}

	// Verify via GetByID.
	loaded, err := us.GetByID(ctx, pool, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if loaded.IsActive {
		t.Error("user should be inactive after SetActive(false)")
	}
}
