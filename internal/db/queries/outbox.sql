-- name: InsertOutbox :one
INSERT INTO outbox (topic, payload)
VALUES ($1, $2)
RETURNING id, topic, payload, status, attempts, last_error, created_at, claimed_at, done_at;

-- name: ClaimPendingOutbox :many
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
RETURNING id, topic, payload, status, attempts, last_error, created_at, claimed_at, done_at;

-- name: AckOutbox :exec
UPDATE outbox
SET status  = 'done',
    done_at = now()
WHERE id = $1;

-- name: NackOutbox :exec
UPDATE outbox
SET status     = 'pending',
    last_error = $2
WHERE id = $1;
