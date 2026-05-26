-- name: UpsertCascadeState :one
INSERT INTO cascade_state (hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at)
VALUES ($1, $2, $3, COALESCE($4, 'pending'::cascade_status), $5, COALESCE($6, 0), $7)
ON CONFLICT (hold_id, custodian_email, instance_id) DO UPDATE
  SET status       = COALESCE(EXCLUDED.status, cascade_state.status),
      last_error   = EXCLUDED.last_error,
      attempts     = EXCLUDED.attempts,
      completed_at = EXCLUDED.completed_at,
      updated_at   = now()
RETURNING id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at;

-- name: GetCascadeStateByHold :many
SELECT id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
FROM cascade_state
WHERE hold_id = $1
ORDER BY custodian_email ASC, instance_id ASC;

-- name: UpdateCascadeStatus :one
UPDATE cascade_state
SET status       = $2,
    last_error   = $3,
    attempts     = attempts + COALESCE($4, 0),
    completed_at = CASE WHEN $2 IN ('completed'::cascade_status, 'failed'::cascade_status) THEN now() ELSE completed_at END,
    updated_at   = now()
WHERE id = $1
RETURNING id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at;
