package legalhold

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/aechrok/warden/internal/domain"
)

// ErrInvalidTransition is returned when a requested cascade status change is
// not allowed from the current state.
var ErrInvalidTransition = errors.New("legalhold: invalid cascade state transition")

// validTransitions defines the allowed (from, to) pairs for cascade state.
// Any state may transition to failed. The reverse path (removed → placing)
// is intentionally forbidden; a new cascade row must be created instead.
var validTransitions = map[domain.CascadeStatus][]domain.CascadeStatus{
	domain.CascadeStatusPending:    {domain.CascadeStatusInProgress, domain.CascadeStatusFailed},
	domain.CascadeStatusInProgress: {domain.CascadeStatusCompleted, domain.CascadeStatusPartial, domain.CascadeStatusFailed},
	domain.CascadeStatusCompleted:  {domain.CascadeStatusFailed},
	domain.CascadeStatusPartial:    {domain.CascadeStatusInProgress, domain.CascadeStatusCompleted, domain.CascadeStatusFailed},
	domain.CascadeStatusFailed:     {domain.CascadeStatusPending},
}

// CascadeStateMachine manages per-custodian, per-instance cascade state
// transitions. Every transition is validated and emits an event to the store.
type CascadeStateMachine struct {
	svc *Service
}

// NewCascadeStateMachine constructs a state machine bound to the given service.
func NewCascadeStateMachine(svc *Service) *CascadeStateMachine {
	return &CascadeStateMachine{svc: svc}
}

// Transition atomically advances a cascade_state row from its current status
// to next within the caller's transaction. Returns ErrInvalidTransition if the
// move is disallowed. lastError is stored when transitioning to failed.
func (m *CascadeStateMachine) Transition(
	ctx context.Context,
	tx pgx.Tx,
	cascadeID uuid.UUID,
	next domain.CascadeStatus,
	lastError string,
) (*domain.CascadeState, error) {
	var cs domain.CascadeState
	err := tx.QueryRow(ctx, `
		SELECT id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
		FROM cascade_state
		WHERE id = $1
		FOR UPDATE
	`, cascadeID).Scan(
		&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID,
		&cs.Status, &cs.LastError, &cs.Attempts,
		&cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: cascade state %s not found", cascadeID)
		}
		return nil, fmt.Errorf("legalhold: load cascade state: %w", err)
	}

	if err := assertTransitionAllowed(cs.Status, next); err != nil {
		return nil, err
	}

	var completedAt any
	if next == domain.CascadeStatusCompleted {
		t := time.Now().UTC()
		completedAt = t
	}

	var errArg any
	if lastError != "" {
		errArg = lastError
	}

	err = tx.QueryRow(ctx, `
		UPDATE cascade_state
		SET status       = $2,
		    last_error   = $3,
		    attempts     = attempts + 1,
		    completed_at = COALESCE($4, completed_at),
		    updated_at   = now()
		WHERE id = $1
		RETURNING id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
	`, cascadeID, string(next), errArg, completedAt).Scan(
		&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID,
		&cs.Status, &cs.LastError, &cs.Attempts,
		&cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("legalhold: update cascade state: %w", err)
	}

	eventType := cascadeEventType(next)
	payload := map[string]any{
		"cascade_id":      cascadeID,
		"hold_id":         cs.HoldID,
		"custodian_email": cs.CustodianEmail,
		"instance_id":     cs.InstanceID,
		"status":          string(next),
	}
	if lastError != "" {
		payload["error"] = lastError
	}

	systemActor := domain.Actor{Type: domain.ActorTypeUser}
	if err := m.svc.appendEvent(ctx, tx, domain.AggregateHold, cs.HoldID, eventType, systemActor, payload); err != nil {
		return nil, err
	}

	return &cs, nil
}

// UpsertPending inserts a cascade_state row in pending status, or returns the
// existing row if one already exists for the (hold, custodian, instance) triple.
func (m *CascadeStateMachine) UpsertPending(
	ctx context.Context,
	tx pgx.Tx,
	holdID uuid.UUID,
	custodianEmail string,
	instanceID uuid.UUID,
) (*domain.CascadeState, error) {
	cs := &domain.CascadeState{}
	err := tx.QueryRow(ctx, `
		INSERT INTO cascade_state (hold_id, custodian_email, instance_id, status)
		VALUES ($1, $2, $3, 'pending')
		ON CONFLICT (hold_id, custodian_email, instance_id) DO UPDATE
		  SET updated_at = cascade_state.updated_at
		RETURNING id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
	`, holdID, custodianEmail, instanceID).Scan(
		&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID,
		&cs.Status, &cs.LastError, &cs.Attempts,
		&cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("legalhold: upsert cascade state: %w", err)
	}
	return cs, nil
}

// LoadByHoldAndCustodian returns all cascade_state rows for a given hold and
// custodian email.
func LoadByHoldAndCustodian(ctx context.Context, tx pgx.Tx, holdID uuid.UUID, email string) ([]*domain.CascadeState, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
		FROM cascade_state
		WHERE hold_id = $1 AND custodian_email = $2
	`, holdID, email)
	if err != nil {
		return nil, fmt.Errorf("legalhold: load cascade by hold+custodian: %w", err)
	}
	defer rows.Close()
	return scanCascadeRows(rows)
}

// LoadActiveByInstance returns all cascade_state rows with status=completed
// for a given integration instance. Used by the reconciler.
func LoadActiveByInstance(ctx context.Context, tx pgx.Tx, instanceID uuid.UUID) ([]*domain.CascadeState, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
		FROM cascade_state
		WHERE instance_id = $1 AND status = 'completed'
	`, instanceID)
	if err != nil {
		return nil, fmt.Errorf("legalhold: load active by instance: %w", err)
	}
	defer rows.Close()
	return scanCascadeRows(rows)
}

func scanCascadeRows(rows pgx.Rows) ([]*domain.CascadeState, error) {
	out := []*domain.CascadeState{}
	for rows.Next() {
		cs := &domain.CascadeState{}
		if err := rows.Scan(
			&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID,
			&cs.Status, &cs.LastError, &cs.Attempts,
			&cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("legalhold: scan cascade state: %w", err)
		}
		out = append(out, cs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legalhold: cascade rows: %w", err)
	}
	return out, nil
}

func assertTransitionAllowed(from, to domain.CascadeStatus) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown source state %q", ErrInvalidTransition, from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
}

func cascadeEventType(status domain.CascadeStatus) string {
	switch status {
	case domain.CascadeStatusInProgress:
		return domain.EventCascadeRequested
	case domain.CascadeStatusCompleted:
		return domain.EventCascadeCompleted
	case domain.CascadeStatusFailed:
		return domain.EventCascadeFailed
	default:
		return domain.EventCascadeUpdated
	}
}
