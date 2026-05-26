// Package breakglass implements Warden's emergency override flow. An
// authorized operator records a reason, the action executes immediately,
// and an incident row is created alongside a breakglass.used event in the
// event store. Post-incident review is captured on the same row.
package breakglass

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/store"
)

// InvokeRequest is the payload supplied by the operator when invoking the
// break-glass override. Reason must be non-empty; the audit trail and the
// admin notification both surface it.
type InvokeRequest struct {
	ActionKey   string
	InstanceID  *uuid.UUID
	TargetEmail string
	Reason      string
}

// Incident is a row from the breakglass_incidents table.
type Incident struct {
	ID          uuid.UUID
	OperatorID  uuid.UUID
	ActionKey   string
	InstanceID  *uuid.UUID
	TargetEmail string
	Reason      string
	ReviewedBy  *uuid.UUID
	ReviewedAt  *time.Time
	ReviewNote  string
	CreatedAt   time.Time
}

// Service records break-glass invocations and exposes the review queue.
type Service struct {
	pool       *pgxpool.Pool
	eventStore *store.EventStore
}

// NewService wires a new break-glass Service. eventStore must be the same
// one used elsewhere in the process so events land in a single stream.
func NewService(pool *pgxpool.Pool, eventStore *store.EventStore) *Service {
	return &Service{pool: pool, eventStore: eventStore}
}

// Invoke records an incident row and a corresponding breakglass.used event.
// The caller is responsible for actually executing the action *after* this
// returns successfully — the incident row should always exist before any
// destructive side effect occurs upstream.
func (s *Service) Invoke(ctx context.Context, actor domain.Actor, req InvokeRequest) (uuid.UUID, error) {
	if s == nil || s.pool == nil || s.eventStore == nil {
		return uuid.Nil, errors.New("breakglass: service not initialized")
	}
	if actor.ID == uuid.Nil {
		return uuid.Nil, errors.New("breakglass: actor id required")
	}
	if strings.TrimSpace(req.ActionKey) == "" {
		return uuid.Nil, errors.New("breakglass: action_key required")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return uuid.Nil, errors.New("breakglass: reason required")
	}

	var incidentID uuid.UUID
	err := runTx(ctx, s.pool, func(tx pgx.Tx) error {
		var instanceID any
		if req.InstanceID != nil {
			instanceID = *req.InstanceID
		}
		var targetEmail any
		if strings.TrimSpace(req.TargetEmail) != "" {
			targetEmail = strings.ToLower(strings.TrimSpace(req.TargetEmail))
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO breakglass_incidents (operator_id, action_key, instance_id, target_email, reason)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, actor.ID, req.ActionKey, instanceID, targetEmail, req.Reason).Scan(&incidentID)
		if err != nil {
			return fmt.Errorf("breakglass: insert incident: %w", err)
		}

		// Aggregate id of the event is the operator's user id so the event
		// stream reflects "this operator used break-glass" in their history.
		version, err := s.eventStore.NextVersion(ctx, tx, domain.AggregateUser, actor.ID)
		if err != nil {
			return fmt.Errorf("breakglass: next version: %w", err)
		}
		payload := map[string]any{
			"incident_id":  incidentID,
			"action_key":   req.ActionKey,
			"target_email": req.TargetEmail,
			"reason":       req.Reason,
		}
		if req.InstanceID != nil {
			payload["instance_id"] = req.InstanceID.String()
		}
		actorIDCopy := actor.ID
		evt := domain.Event{
			AggregateType: domain.AggregateUser,
			AggregateID:   actor.ID,
			Version:       version,
			Type:          domain.EventBreakGlassUsed,
			Payload:       payload,
			ActorID:       &actorIDCopy,
			ActorType:     actor.Type,
		}
		if _, err := s.eventStore.Append(ctx, tx, evt); err != nil {
			return fmt.Errorf("breakglass: append event: %w", err)
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return incidentID, nil
}

// ListIncidents returns recent incidents, newest first, paginated. limit is
// clamped to a sane upper bound to keep responses small.
func (s *Service) ListIncidents(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]Incident, error) {
	if pool == nil {
		return nil, errors.New("breakglass: list: nil pool")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := pool.Query(ctx, `
		SELECT id, operator_id, action_key, instance_id, target_email, reason,
		       reviewed_by, reviewed_at, COALESCE(review_note, ''), created_at
		FROM breakglass_incidents
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("breakglass: list: %w", err)
	}
	defer rows.Close()

	out := []Incident{}
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("breakglass: list rows: %w", err)
	}
	return out, nil
}

// GetIncident loads a single incident by id.
func (s *Service) GetIncident(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Incident, error) {
	if pool == nil {
		return nil, errors.New("breakglass: get: nil pool")
	}
	row := pool.QueryRow(ctx, `
		SELECT id, operator_id, action_key, instance_id, target_email, reason,
		       reviewed_by, reviewed_at, COALESCE(review_note, ''), created_at
		FROM breakglass_incidents
		WHERE id = $1
	`, id)
	i, err := scanIncident(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("breakglass: incident %s not found", id)
		}
		return nil, err
	}
	return &i, nil
}

// ReviewIncident records the review action: which operator reviewed, when,
// and an optional free-form note. Also appends a breakglass.reviewed event.
// Returns an error if the incident has already been reviewed.
func (s *Service) ReviewIncident(ctx context.Context, pool *pgxpool.Pool, id, reviewerID uuid.UUID, note string) error {
	if pool == nil {
		return errors.New("breakglass: review: nil pool")
	}
	if id == uuid.Nil || reviewerID == uuid.Nil {
		return errors.New("breakglass: review: nil id")
	}
	return runTx(ctx, pool, func(tx pgx.Tx) error {
		var (
			operatorID    uuid.UUID
			alreadyReview *uuid.UUID
		)
		err := tx.QueryRow(ctx, `
			SELECT operator_id, reviewed_by
			FROM breakglass_incidents
			WHERE id = $1
			FOR UPDATE
		`, id).Scan(&operatorID, &alreadyReview)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("breakglass: incident %s not found", id)
			}
			return fmt.Errorf("breakglass: review lookup: %w", err)
		}
		if alreadyReview != nil {
			return errors.New("breakglass: incident already reviewed")
		}

		if _, err := tx.Exec(ctx, `
			UPDATE breakglass_incidents
			SET reviewed_by = $2,
			    reviewed_at = now(),
			    review_note = $3
			WHERE id = $1
		`, id, reviewerID, note); err != nil {
			return fmt.Errorf("breakglass: review update: %w", err)
		}

		version, err := s.eventStore.NextVersion(ctx, tx, domain.AggregateUser, operatorID)
		if err != nil {
			return fmt.Errorf("breakglass: review version: %w", err)
		}
		reviewerCopy := reviewerID
		evt := domain.Event{
			AggregateType: domain.AggregateUser,
			AggregateID:   operatorID,
			Version:       version,
			Type:          domain.EventBreakGlassReviewed,
			Payload: map[string]any{
				"incident_id": id,
				"reviewer_id": reviewerID,
				"note":        note,
			},
			ActorID:   &reviewerCopy,
			ActorType: domain.ActorTypeUser,
		}
		if _, err := s.eventStore.Append(ctx, tx, evt); err != nil {
			return fmt.Errorf("breakglass: review event: %w", err)
		}
		return nil
	})
}

// LastInvocation returns the timestamp of the most recent break-glass
// invocation by the supplied operator, or nil if they have never used it.
// Used by the breakglass_cooldown PBAC policy.
func (s *Service) LastInvocation(ctx context.Context, pool *pgxpool.Pool, operatorID uuid.UUID) (*time.Time, error) {
	if pool == nil {
		return nil, errors.New("breakglass: last invocation: nil pool")
	}
	var ts *time.Time
	err := pool.QueryRow(ctx, `
		SELECT MAX(created_at)
		FROM breakglass_incidents
		WHERE operator_id = $1
	`, operatorID).Scan(&ts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("breakglass: last invocation: %w", err)
	}
	return ts, nil
}

// scanIncident reads one row regardless of whether it came from QueryRow
// (pgx.Row) or Query (pgx.Rows).
type rowScanner interface {
	Scan(dest ...any) error
}

func scanIncident(r rowScanner) (Incident, error) {
	var (
		i           Incident
		instanceID  *uuid.UUID
		targetEmail *string
		reviewedBy  *uuid.UUID
	)
	err := r.Scan(
		&i.ID,
		&i.OperatorID,
		&i.ActionKey,
		&instanceID,
		&targetEmail,
		&i.Reason,
		&reviewedBy,
		&i.ReviewedAt,
		&i.ReviewNote,
		&i.CreatedAt,
	)
	if err != nil {
		return Incident{}, err
	}
	if instanceID != nil {
		i.InstanceID = instanceID
	}
	if targetEmail != nil {
		i.TargetEmail = *targetEmail
	}
	if reviewedBy != nil {
		i.ReviewedBy = reviewedBy
	}
	return i, nil
}

func runTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("breakglass: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
