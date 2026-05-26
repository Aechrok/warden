// Package legalhold implements the legal hold lifecycle, cascade state machine,
// River workers, and hold template CRUD for Warden.
package legalhold

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/internal/store"
)

// Service manages the legal hold lifecycle. All state-mutating methods accept a
// caller-provided pgx.Tx so they can be composed with other writes atomically.
type Service struct {
	pool        *pgxpool.Pool
	eventStore  *store.EventStore
	outboxStore *store.OutboxStore
	registry    *plugin.Registry
	riverClient *river.Client[pgx.Tx]
}

// NewService constructs a Service. If registry is nil the global plugin registry
// is used. riverClient may be nil during construction and set later via
// SetRiverClient — workers require it, but template CRUD does not.
func NewService(
	pool *pgxpool.Pool,
	eventStore *store.EventStore,
	outboxStore *store.OutboxStore,
	registry *plugin.Registry,
) *Service {
	if registry == nil {
		registry = plugin.NewRegistry()
	}
	return &Service{
		pool:        pool,
		eventStore:  eventStore,
		outboxStore: outboxStore,
		registry:    registry,
	}
}

// SetRiverClient wires the River client used for direct job insertion by
// workers. Agent 5 calls this after constructing the full River client.
func (s *Service) SetRiverClient(rc *river.Client[pgx.Tx]) {
	s.riverClient = rc
}

// NewRiverClient constructs a River client bound to the service's pool and
// registers all legalhold workers. Agent 5 can call this or build its own.
func (s *Service) NewRiverClient() (*river.Client[pgx.Tx], error) {
	workers := NewWorkers(s)
	rc, err := river.NewClient(riverpgxv5.New(s.pool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(2*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return ReconcileHoldsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("legalhold: new river client: %w", err)
	}
	s.riverClient = rc
	return rc, nil
}

// CreateHoldParams carries the fields required to open a new hold.
type CreateHoldParams struct {
	Name        string
	Description string
	TemplateID  *uuid.UUID
	ExpiresAt   *time.Time
	Actor       domain.Actor
}

// CreateHold opens a new legal hold in status=active. The caller must provide
// an open transaction; the hold row and its opening event are committed
// together. An ExpireHoldJob is enqueued in the same transaction when
// ExpiresAt is set.
func (s *Service) CreateHold(ctx context.Context, tx pgx.Tx, params CreateHoldParams) (*domain.Hold, error) {
	if strings.TrimSpace(params.Name) == "" {
		return nil, errors.New("legalhold: hold name required")
	}

	hold := &domain.Hold{}
	err := tx.QueryRow(ctx, `
		INSERT INTO legal_holds (name, description, template_id, placed_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
	`,
		params.Name,
		nullText(params.Description),
		params.TemplateID,
		nullUUID(params.Actor.ID),
		params.ExpiresAt,
	).Scan(
		&hold.ID,
		&hold.Name,
		&hold.Description,
		&hold.TemplateID,
		&hold.Status,
		&hold.PlacedBy,
		&hold.ExpiresAt,
		&hold.ReleasedAt,
		&hold.CreatedAt,
		&hold.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("legalhold: create hold: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, hold.ID, domain.EventHoldCreated, params.Actor, map[string]any{
		"hold_id":     hold.ID,
		"name":        hold.Name,
		"description": hold.Description,
		"template_id": hold.TemplateID,
		"expires_at":  hold.ExpiresAt,
	}); err != nil {
		return nil, err
	}

	if hold.ExpiresAt != nil {
		if _, err := s.outboxStore.Enqueue(ctx, tx, "river.expire_hold", ExpireHoldArgs{
			HoldID:      hold.ID,
			ScheduledAt: *hold.ExpiresAt,
		}); err != nil {
			return nil, fmt.Errorf("legalhold: enqueue expire job: %w", err)
		}
	}

	return hold, nil
}

// AddCustodianParams carries fields required to add a custodian to a hold.
type AddCustodianParams struct {
	HoldID string
	Email  string
	Actor  domain.Actor
}

// AddCustodian adds a custodian to an active hold and enqueues a cascade place
// job via the transactional outbox. Returns an error if the hold is not active
// or if the custodian is already on the hold.
func (s *Service) AddCustodian(ctx context.Context, tx pgx.Tx, holdID uuid.UUID, email string, actor domain.Actor) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("legalhold: custodian email required")
	}

	hold, err := s.getHoldTx(ctx, tx, holdID)
	if err != nil {
		return err
	}
	if hold.Status != domain.HoldStatusActive {
		return fmt.Errorf("legalhold: hold %s is not active (status=%s)", holdID, hold.Status)
	}

	var custodianID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO legal_hold_custodians (hold_id, email, added_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (hold_id, email) DO UPDATE
		  SET removed_at = NULL, removed_by = NULL
		RETURNING id
	`, holdID, email, nullUUID(actor.ID)).Scan(&custodianID)
	if err != nil {
		return fmt.Errorf("legalhold: add custodian: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, holdID, domain.EventCustodianAdded, actor, map[string]any{
		"custodian_id":    custodianID,
		"custodian_email": email,
	}); err != nil {
		return err
	}

	if _, err := s.outboxStore.Enqueue(ctx, tx, "river.cascade_place", CascadePlaceArgs{
		HoldID:         holdID,
		CustodianID:    custodianID,
		CustodianEmail: email,
	}); err != nil {
		return fmt.Errorf("legalhold: enqueue cascade place: %w", err)
	}

	return nil
}

// RemoveCustodian soft-deletes a custodian from a hold and enqueues a cascade
// remove job. The custodian record is preserved for audit purposes.
func (s *Service) RemoveCustodian(ctx context.Context, tx pgx.Tx, holdID, custodianID uuid.UUID, actor domain.Actor) error {
	var email string
	err := tx.QueryRow(ctx, `
		UPDATE legal_hold_custodians
		SET removed_at = now(), removed_by = $3
		WHERE hold_id = $1 AND id = $2 AND removed_at IS NULL
		RETURNING email
	`, holdID, custodianID, nullUUID(actor.ID)).Scan(&email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("legalhold: custodian %s not found on hold %s or already removed", custodianID, holdID)
		}
		return fmt.Errorf("legalhold: remove custodian: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, holdID, domain.EventCustodianRemoved, actor, map[string]any{
		"custodian_id":    custodianID,
		"custodian_email": email,
	}); err != nil {
		return err
	}

	if _, err := s.outboxStore.Enqueue(ctx, tx, "river.cascade_remove", CascadeRemoveArgs{
		HoldID:         holdID,
		CustodianID:    custodianID,
		CustodianEmail: email,
	}); err != nil {
		return fmt.Errorf("legalhold: enqueue cascade remove: %w", err)
	}

	return nil
}

// ReleaseHold marks a hold as released and enqueues cascade remove jobs for
// every active custodian. The reason is recorded on the hold row.
func (s *Service) ReleaseHold(ctx context.Context, tx pgx.Tx, holdID uuid.UUID, reason string, actor domain.Actor) error {
	var releasedAt time.Time
	err := tx.QueryRow(ctx, `
		UPDATE legal_holds
		SET status = 'released', released_at = now()
		WHERE id = $1 AND status = 'active'
		RETURNING released_at
	`, holdID).Scan(&releasedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("legalhold: hold %s not found or not active", holdID)
		}
		return fmt.Errorf("legalhold: release hold: %w", err)
	}

	if err := s.appendEvent(ctx, tx, domain.AggregateHold, holdID, domain.EventHoldReleased, actor, map[string]any{
		"reason":      reason,
		"released_at": releasedAt,
	}); err != nil {
		return err
	}

	return s.enqueueRemoveForAllCustodians(ctx, tx, holdID)
}

// ExpireHold is called by the ExpireHoldWorker when a hold's ExpiresAt fires.
// It marks the hold expired and enqueues cascade remove jobs for active custodians.
func (s *Service) ExpireHold(ctx context.Context, tx pgx.Tx, holdID uuid.UUID) error {
	tag, err := tx.Exec(ctx, `
		UPDATE legal_holds
		SET status = 'expired', released_at = now()
		WHERE id = $1 AND status = 'active'
	`, holdID)
	if err != nil {
		return fmt.Errorf("legalhold: expire hold: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	systemActor := domain.Actor{Type: domain.ActorTypeUser}
	if err := s.appendEvent(ctx, tx, domain.AggregateHold, holdID, domain.EventHoldExpired, systemActor, map[string]any{
		"hold_id": holdID,
	}); err != nil {
		return err
	}

	return s.enqueueRemoveForAllCustodians(ctx, tx, holdID)
}

// GetHold loads a single hold by ID.
func (s *Service) GetHold(ctx context.Context, pool *pgxpool.Pool, holdID uuid.UUID) (*domain.Hold, error) {
	hold := &domain.Hold{}
	err := pool.QueryRow(ctx, `
		SELECT id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
		FROM legal_holds WHERE id = $1
	`, holdID).Scan(
		&hold.ID, &hold.Name, &hold.Description, &hold.TemplateID,
		&hold.Status, &hold.PlacedBy, &hold.ExpiresAt, &hold.ReleasedAt,
		&hold.CreatedAt, &hold.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: hold %s not found", holdID)
		}
		return nil, fmt.Errorf("legalhold: get hold: %w", err)
	}
	return hold, nil
}

// ListHoldsFilter controls which holds are returned by ListHolds.
type ListHoldsFilter struct {
	Status *domain.HoldStatus
}

// ListHolds returns holds matching the filter, newest first.
func (s *Service) ListHolds(ctx context.Context, pool *pgxpool.Pool, filter ListHoldsFilter) ([]*domain.Hold, error) {
	query := `
		SELECT id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
		FROM legal_holds
	`
	args := []any{}
	if filter.Status != nil {
		query += " WHERE status = $1"
		args = append(args, string(*filter.Status))
	}
	query += " ORDER BY created_at DESC"

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("legalhold: list holds: %w", err)
	}
	defer rows.Close()

	out := []*domain.Hold{}
	for rows.Next() {
		h := &domain.Hold{}
		if err := rows.Scan(
			&h.ID, &h.Name, &h.Description, &h.TemplateID,
			&h.Status, &h.PlacedBy, &h.ExpiresAt, &h.ReleasedAt,
			&h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("legalhold: scan hold: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("legalhold: list holds rows: %w", err)
	}
	return out, nil
}

// enqueueRemoveForAllCustodians enqueues CascadeRemoveArgs for every
// currently active (non-removed) custodian on the hold.
func (s *Service) enqueueRemoveForAllCustodians(ctx context.Context, tx pgx.Tx, holdID uuid.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT id, email FROM legal_hold_custodians
		WHERE hold_id = $1 AND removed_at IS NULL
	`, holdID)
	if err != nil {
		return fmt.Errorf("legalhold: list custodians for remove: %w", err)
	}
	defer rows.Close()

	type custodian struct {
		id    uuid.UUID
		email string
	}
	var custodians []custodian
	for rows.Next() {
		var c custodian
		if err := rows.Scan(&c.id, &c.email); err != nil {
			return fmt.Errorf("legalhold: scan custodian: %w", err)
		}
		custodians = append(custodians, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("legalhold: custodian rows: %w", err)
	}

	for _, c := range custodians {
		if _, err := s.outboxStore.Enqueue(ctx, tx, "river.cascade_remove", CascadeRemoveArgs{
			HoldID:         holdID,
			CustodianID:    c.id,
			CustodianEmail: c.email,
		}); err != nil {
			return fmt.Errorf("legalhold: enqueue cascade remove for %s: %w", c.email, err)
		}
	}
	return nil
}

// appendEvent is a helper that calls NextVersion + Append in one shot.
func (s *Service) appendEvent(
	ctx context.Context,
	tx pgx.Tx,
	aggType domain.AggregateType,
	aggID uuid.UUID,
	eventType string,
	actor domain.Actor,
	payload map[string]any,
) error {
	version, err := s.eventStore.NextVersion(ctx, tx, aggType, aggID)
	if err != nil {
		return fmt.Errorf("legalhold: next version: %w", err)
	}
	var actorID *uuid.UUID
	if actor.ID != uuid.Nil {
		id := actor.ID
		actorID = &id
	}
	evt := domain.Event{
		AggregateType: aggType,
		AggregateID:   aggID,
		Version:       version,
		Type:          eventType,
		Payload:       payload,
		ActorID:       actorID,
		ActorType:     actor.Type,
	}
	if _, err := s.eventStore.Append(ctx, tx, evt); err != nil {
		return fmt.Errorf("legalhold: append event %s: %w", eventType, err)
	}
	return nil
}

// getHoldTx loads a hold within the provided transaction (for SELECT FOR UPDATE).
func (s *Service) getHoldTx(ctx context.Context, tx pgx.Tx, holdID uuid.UUID) (*domain.Hold, error) {
	hold := &domain.Hold{}
	err := tx.QueryRow(ctx, `
		SELECT id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
		FROM legal_holds WHERE id = $1 FOR UPDATE
	`, holdID).Scan(
		&hold.ID, &hold.Name, &hold.Description, &hold.TemplateID,
		&hold.Status, &hold.PlacedBy, &hold.ExpiresAt, &hold.ReleasedAt,
		&hold.CreatedAt, &hold.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("legalhold: hold %s not found", holdID)
		}
		return nil, fmt.Errorf("legalhold: get hold tx: %w", err)
	}
	return hold, nil
}

// nullUUID returns nil when id is the zero value, so pgx stores NULL.
func nullUUID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// nullText returns nil when s is empty, so pgx stores NULL.
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}
