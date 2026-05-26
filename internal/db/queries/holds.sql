-- name: CreateHold :one
INSERT INTO legal_holds (name, description, template_id, status, placed_by, expires_at)
VALUES ($1, $2, $3, COALESCE($4, 'active'::hold_status), $5, $6)
RETURNING id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at;

-- name: GetHoldByID :one
SELECT id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
FROM legal_holds
WHERE id = $1;

-- name: ListActiveHolds :many
SELECT id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at
FROM legal_holds
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateHoldStatus :one
UPDATE legal_holds
SET status      = $2,
    released_at = CASE WHEN $2 IN ('released'::hold_status, 'expired'::hold_status) THEN now() ELSE released_at END,
    updated_at  = now()
WHERE id = $1
RETURNING id, name, description, template_id, status, placed_by, expires_at, released_at, created_at, updated_at;

-- name: CreateCustodian :one
INSERT INTO legal_hold_custodians (hold_id, email, added_by)
VALUES ($1, $2, $3)
RETURNING id, hold_id, email, added_by, removed_at, removed_by, created_at;

-- name: ListCustodiansByHold :many
SELECT id, hold_id, email, added_by, removed_at, removed_by, created_at
FROM legal_hold_custodians
WHERE hold_id = $1
ORDER BY created_at ASC;

-- name: GetActiveCustodian :one
SELECT id, hold_id, email, added_by, removed_at, removed_by, created_at
FROM legal_hold_custodians
WHERE hold_id = $1
  AND email = $2
  AND removed_at IS NULL;

-- name: RemoveCustodian :one
UPDATE legal_hold_custodians
SET removed_at = now(),
    removed_by = $2
WHERE id = $1
  AND removed_at IS NULL
RETURNING id, hold_id, email, added_by, removed_at, removed_by, created_at;

-- name: CountActiveHoldsForEmail :one
SELECT COUNT(*)::bigint AS count
FROM legal_hold_custodians c
JOIN legal_holds h ON h.id = c.hold_id
WHERE c.email = $1
  AND c.removed_at IS NULL
  AND h.status = 'active';
