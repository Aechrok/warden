package legalhold_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/legalhold"
	"github.com/aechrok/warden/internal/store"
)

func TestTemplates_CRUD(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	actor := systemActor()
	es := store.NewEventStore()
	ob := store.NewOutboxStore()
	svc := legalhold.NewService(pool, es, ob, nil)

	// CREATE
	tx1 := beginTx(t, pool)
	tpl, err := svc.CreateTemplate(ctx, tx1, actor, legalhold.CreateTemplateParams{
		Name:         "Litigation Template",
		Description:  "Standard litigation hold",
		ProviderGlob: "google_vault",
	})
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}
	if tpl.ID == uuid.Nil {
		t.Fatal("expected non-nil template ID")
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit create: %v", err)
	}

	// GET
	loaded, err := svc.GetTemplate(ctx, pool, tpl.ID)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if loaded.Name != "Litigation Template" {
		t.Errorf("Name = %q, want %q", loaded.Name, "Litigation Template")
	}

	// LIST
	templates, err := svc.ListTemplates(ctx, pool)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	found := false
	for _, tl := range templates {
		if tl.ID == tpl.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created template not found in ListTemplates")
	}

	// UPDATE
	tx2 := beginTx(t, pool)
	updated, err := svc.UpdateTemplate(ctx, tx2, actor, tpl.ID, legalhold.UpdateTemplateParams{
		Name:         "Updated Template",
		ProviderGlob: "*",
	})
	if err != nil {
		t.Fatalf("UpdateTemplate: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit update: %v", err)
	}
	if updated.Name != "Updated Template" {
		t.Errorf("updated Name = %q, want %q", updated.Name, "Updated Template")
	}

	// DELETE
	tx3 := beginTx(t, pool)
	if err := svc.DeleteTemplate(ctx, tx3, actor, tpl.ID); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}
	if err := tx3.Commit(ctx); err != nil {
		t.Fatalf("commit delete: %v", err)
	}

	// Verify deleted.
	_, err = svc.GetTemplate(ctx, pool, tpl.ID)
	if err == nil {
		t.Error("expected error loading deleted template")
	}
}

func TestCascadeStateMachine_invalidTransition(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	actor := systemActor()
	es := store.NewEventStore()
	ob := store.NewOutboxStore()
	svc := legalhold.NewService(pool, es, ob, nil)
	csm := legalhold.NewCascadeStateMachine(svc)
	instanceID := insertFakeInstance(t, pool, "test_sm")

	// Create a hold and a custodian so we have a cascade_state row.
	tx0 := beginTx(t, pool)
	hold, err := svc.CreateHold(ctx, tx0, legalhold.CreateHoldParams{Name: "SM Test Hold", Actor: actor})
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	if err := tx0.Commit(ctx); err != nil {
		t.Fatalf("commit hold: %v", err)
	}

	// Upsert a pending cascade row.
	tx1 := beginTx(t, pool)
	cs, err := csm.UpsertPending(ctx, tx1, hold.ID, "sm_test@example.com", instanceID)
	if err != nil {
		t.Fatalf("UpsertPending: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit upsert: %v", err)
	}

	// Pending → Completed is NOT a valid transition (must go through InProgress).
	tx2 := beginTx(t, pool)
	_, err = csm.Transition(ctx, tx2, cs.ID, domain.CascadeStatusCompleted, "")
	if err == nil {
		t.Error("expected error for invalid transition pending→completed")
	}
}
