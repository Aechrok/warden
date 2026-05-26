// Package store provides transactional persistence for the event store and
// the transactional outbox. Both stores are designed to be composed with
// other writes inside a single pgx.Tx so callers can guarantee atomicity
// between domain state changes and the audit / dispatch trail.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/aechrok/warden/internal/domain"
)

// ErrVersionConflict is returned by EventStore.Append when the optimistic
// concurrency check fails: another writer has already appended the same
// (aggregate_type, aggregate_id, version) triple.
var ErrVersionConflict = errors.New("event store: version conflict")

// EventStore is the append-only event log. All writes must occur inside a
// caller-provided pgx.Tx so they commit atomically with the aggregate state
// change they describe.
type EventStore struct{}

// NewEventStore constructs an EventStore. There is no per-instance state;
// the value exists purely to provide a method namespace.
func NewEventStore() *EventStore {
	return &EventStore{}
}

// NextVersion returns the next version number to use when appending to the
// given aggregate stream. If the stream has no prior events, returns 1.
func (s *EventStore) NextVersion(ctx context.Context, tx pgx.Tx, aggregateType domain.AggregateType, aggregateID uuid.UUID) (int, error) {
	var current int
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0)::int
		FROM events
		WHERE aggregate_type = $1
		  AND aggregate_id = $2
	`, string(aggregateType), aggregateID).Scan(&current)
	if err != nil {
		return 0, fmt.Errorf("event store: next version: %w", err)
	}
	return current + 1, nil
}

// Append writes a single event to the log. Version must be the value
// returned by a prior NextVersion call (or computed equivalently). If the
// unique (aggregate_type, aggregate_id, version) constraint fires, returns
// ErrVersionConflict so callers can retry.
func (s *EventStore) Append(ctx context.Context, tx pgx.Tx, e domain.Event) (domain.Event, error) {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return domain.Event{}, fmt.Errorf("event store: marshal payload: %w", err)
	}
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}

	var actorID any
	if e.ActorID != nil {
		actorID = *e.ActorID
	}
	var actorType any
	if e.ActorType != "" {
		actorType = string(e.ActorType)
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO events (
			aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		RETURNING id, created_at
	`,
		string(e.AggregateType),
		e.AggregateID,
		e.Version,
		e.Type,
		string(payload),
		actorID,
		actorType,
	)

	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Event{}, ErrVersionConflict
		}
		return domain.Event{}, fmt.Errorf("event store: append: %w", err)
	}
	return e, nil
}

// LoadByAggregate returns all events for the given aggregate in version order.
func (s *EventStore) LoadByAggregate(ctx context.Context, q pgx.Tx, aggregateType domain.AggregateType, aggregateID uuid.UUID) ([]domain.Event, error) {
	rows, err := q.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at
		FROM events
		WHERE aggregate_type = $1
		  AND aggregate_id = $2
		ORDER BY version ASC
	`, string(aggregateType), aggregateID)
	if err != nil {
		return nil, fmt.Errorf("event store: load by aggregate: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]domain.Event, error) {
	out := []domain.Event{}
	for rows.Next() {
		var (
			e            domain.Event
			aggType      string
			payloadBytes []byte
			actorID      *uuid.UUID
			actorType    *string
		)
		if err := rows.Scan(
			&e.ID,
			&aggType,
			&e.AggregateID,
			&e.Version,
			&e.Type,
			&payloadBytes,
			&actorID,
			&actorType,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("event store: scan: %w", err)
		}
		e.AggregateType = domain.AggregateType(aggType)
		if actorID != nil {
			e.ActorID = actorID
		}
		if actorType != nil {
			e.ActorType = domain.ActorType(*actorType)
		}
		if len(payloadBytes) > 0 {
			if err := json.Unmarshal(payloadBytes, &e.Payload); err != nil {
				return nil, fmt.Errorf("event store: unmarshal payload: %w", err)
			}
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("event store: rows: %w", err)
	}
	return out, nil
}
