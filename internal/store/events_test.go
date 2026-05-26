package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/store"
	"github.com/aechrok/warden/internal/testutil"
)

func TestEventStore_AppendAndLoad(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	es := store.NewEventStore()

	aggID := uuid.New()

	// Append two events to the same aggregate.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}

	v1, err := es.NextVersion(ctx, tx1, domain.AggregateHold, aggID)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("NextVersion 1: %v", err)
	}
	e1, err := es.Append(ctx, tx1, domain.Event{
		AggregateType: domain.AggregateHold,
		AggregateID:   aggID,
		Version:       v1,
		Type:          domain.EventHoldCreated,
		Payload:       map[string]any{"name": "Test Hold"},
	})
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Append e1: %v", err)
	}

	v2, err := es.NextVersion(ctx, tx1, domain.AggregateHold, aggID)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("NextVersion 2: %v", err)
	}
	_, err = es.Append(ctx, tx1, domain.Event{
		AggregateType: domain.AggregateHold,
		AggregateID:   aggID,
		Version:       v2,
		Type:          domain.EventHoldUpdated,
		Payload:       map[string]any{"change": "custodian_added"},
	})
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Append e2: %v", err)
	}

	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	// Load events and verify version ordering.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	events, err := es.LoadByAggregate(ctx, tx2, domain.AggregateHold, aggID)
	if err != nil {
		t.Fatalf("LoadByAggregate: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Version != 1 {
		t.Errorf("event[0].Version = %d, want 1", events[0].Version)
	}
	if events[1].Version != 2 {
		t.Errorf("event[1].Version = %d, want 2", events[1].Version)
	}
	if events[0].ID == uuid.Nil {
		t.Error("event[0].ID should be set")
	}
	_ = e1
}

func TestEventStore_VersionConflict(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	es := store.NewEventStore()

	aggID := uuid.New()

	// Append the first event normally.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	v1, err := es.NextVersion(ctx, tx1, domain.AggregateHold, aggID)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("NextVersion: %v", err)
	}
	if _, err := es.Append(ctx, tx1, domain.Event{
		AggregateType: domain.AggregateHold,
		AggregateID:   aggID,
		Version:       v1,
		Type:          domain.EventHoldCreated,
		Payload:       map[string]any{"name": "Conflict Test"},
	}); err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Append first: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}

	// Attempt to append a second event with the same version (conflict).
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	_, conflictErr := es.Append(ctx, tx2, domain.Event{
		AggregateType: domain.AggregateHold,
		AggregateID:   aggID,
		Version:       v1, // reuse version 1 to trigger conflict
		Type:          domain.EventHoldUpdated,
		Payload:       map[string]any{"name": "Conflict"},
	})
	if !errors.Is(conflictErr, store.ErrVersionConflict) {
		t.Errorf("expected ErrVersionConflict, got %v", conflictErr)
	}
}
