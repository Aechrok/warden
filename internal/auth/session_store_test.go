package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/testutil"
)

func TestSessionStore_CreateValidate(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	ss := auth.NewSessionStore()
	us := auth.NewUserStore()

	// Create a user first.
	tx0, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	user, err := us.GetOrCreate(ctx, tx0, "sess_validate@example.com", "Session User")
	if err != nil {
		_ = tx0.Rollback(ctx)
		t.Fatalf("GetOrCreate: %v", err)
	}
	if err := tx0.Commit(ctx); err != nil {
		t.Fatalf("commit user: %v", err)
	}

	// Create session.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	token, err := ss.Create(ctx, tx1, user.ID, time.Hour)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Create: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit session: %v", err)
	}

	// Validate with correct token.
	data, err := ss.Validate(ctx, pool, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if data.UserID != user.ID {
		t.Errorf("UserID = %v, want %v", data.UserID, user.ID)
	}
}

func TestSessionStore_Validate_expired(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	ss := auth.NewSessionStore()
	us := auth.NewUserStore()

	tx0, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	user, err := us.GetOrCreate(ctx, tx0, "sess_expired@example.com", "Expired User")
	if err != nil {
		_ = tx0.Rollback(ctx)
		t.Fatalf("GetOrCreate: %v", err)
	}
	if err := tx0.Commit(ctx); err != nil {
		t.Fatalf("commit user: %v", err)
	}

	// Create session with 1-nanosecond TTL (will be expired immediately).
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	token, err := ss.Create(ctx, tx1, user.ID, time.Nanosecond)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Create: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Short sleep to ensure the token is expired.
	time.Sleep(5 * time.Millisecond)

	_, err = ss.Validate(ctx, pool, token)
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound for expired session, got %v", err)
	}
}

func TestSessionStore_Delete(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	ss := auth.NewSessionStore()
	us := auth.NewUserStore()

	tx0, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	user, err := us.GetOrCreate(ctx, tx0, "sess_delete@example.com", "Delete User")
	if err != nil {
		_ = tx0.Rollback(ctx)
		t.Fatalf("GetOrCreate: %v", err)
	}
	if err := tx0.Commit(ctx); err != nil {
		t.Fatalf("commit user: %v", err)
	}

	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	token, err := ss.Create(ctx, tx1, user.ID, time.Hour)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("Create: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Delete the session.
	if err := ss.Delete(ctx, pool, token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = ss.Validate(ctx, pool, token)
	if !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound after Delete, got %v", err)
	}
}
