package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OutboxStatus mirrors the outbox_status enum in the database.
type OutboxStatus string

const (
	OutboxStatusPending OutboxStatus = "pending"
	OutboxStatusClaimed OutboxStatus = "claimed"
	OutboxStatusDone    OutboxStatus = "done"
)

// OutboxMessage is one row from the transactional outbox.
type OutboxMessage struct {
	ID        uuid.UUID
	Topic     string
	Payload   []byte
	Status    OutboxStatus
	Attempts  int
	LastError string
	CreatedAt time.Time
	ClaimedAt *time.Time
	DoneAt    *time.Time
}

// OutboxStore manages the transactional outbox. Enqueue must run inside the
// same transaction as the domain write that produced the message; ClaimBatch
// / Ack / Nack are typically run by a dispatcher worker against its own tx.
type OutboxStore struct{}

// NewOutboxStore constructs an OutboxStore.
func NewOutboxStore() *OutboxStore {
	return &OutboxStore{}
}

// Enqueue serializes payload to JSON and inserts a new pending outbox row
// inside the provided transaction. Payload may be any JSON-marshalable value.
func (s *OutboxStore) Enqueue(ctx context.Context, tx pgx.Tx, topic string, payload any) (OutboxMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return OutboxMessage{}, fmt.Errorf("outbox: marshal payload: %w", err)
	}

	var msg OutboxMessage
	err = tx.QueryRow(ctx, `
		INSERT INTO outbox (topic, payload)
		VALUES ($1, $2::jsonb)
		RETURNING id, topic, payload, status::text, attempts, COALESCE(last_error, ''), created_at, claimed_at, done_at
	`, topic, string(raw)).Scan(
		&msg.ID,
		&msg.Topic,
		&msg.Payload,
		(*string)(&msg.Status),
		&msg.Attempts,
		&msg.LastError,
		&msg.CreatedAt,
		&msg.ClaimedAt,
		&msg.DoneAt,
	)
	if err != nil {
		return OutboxMessage{}, fmt.Errorf("outbox: enqueue: %w", err)
	}
	return msg, nil
}

// ClaimBatch atomically claims up to limit pending messages, marking them
// "claimed" and incrementing attempts. Uses SELECT ... FOR UPDATE SKIP LOCKED
// so multiple dispatchers can run concurrently without contention.
func (s *OutboxStore) ClaimBatch(ctx context.Context, tx pgx.Tx, limit int) ([]OutboxMessage, error) {
	if limit <= 0 {
		limit = 1
	}
	rows, err := tx.Query(ctx, `
		UPDATE outbox
		SET status     = 'claimed',
		    claimed_at = now(),
		    attempts   = attempts + 1
		WHERE id IN (
		  SELECT id
		  FROM outbox
		  WHERE status = 'pending'
		  ORDER BY created_at ASC
		  FOR UPDATE SKIP LOCKED
		  LIMIT $1
		)
		RETURNING id, topic, payload, status::text, attempts, COALESCE(last_error, ''), created_at, claimed_at, done_at
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim batch: %w", err)
	}
	defer rows.Close()

	out := []OutboxMessage{}
	for rows.Next() {
		var m OutboxMessage
		if err := rows.Scan(
			&m.ID,
			&m.Topic,
			&m.Payload,
			(*string)(&m.Status),
			&m.Attempts,
			&m.LastError,
			&m.CreatedAt,
			&m.ClaimedAt,
			&m.DoneAt,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan claimed: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: claim rows: %w", err)
	}
	return out, nil
}

// Ack marks a claimed message as done. Idempotent: a message already done
// remains done.
func (s *OutboxStore) Ack(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE outbox
		SET status  = 'done',
		    done_at = now()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("outbox: ack: %w", err)
	}
	return nil
}

// Nack records a failure and returns the message to the pending pool for a
// future ClaimBatch. Attempts was already incremented when the message was
// claimed.
func (s *OutboxStore) Nack(ctx context.Context, tx pgx.Tx, id uuid.UUID, cause string) error {
	_, err := tx.Exec(ctx, `
		UPDATE outbox
		SET status     = 'pending',
		    last_error = $2,
		    claimed_at = NULL
		WHERE id = $1
	`, id, cause)
	if err != nil {
		return fmt.Errorf("outbox: nack: %w", err)
	}
	return nil
}
