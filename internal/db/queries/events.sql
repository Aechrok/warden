-- name: InsertEvent :one
INSERT INTO events (
  aggregate_type,
  aggregate_id,
  version,
  type,
  payload,
  actor_id,
  actor_type
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at;

-- name: GetEventsByAggregate :many
SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at
FROM events
WHERE aggregate_type = $1
  AND aggregate_id = $2
ORDER BY version ASC;

-- name: GetEventsSince :many
SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at
FROM events
WHERE created_at >= $1
ORDER BY created_at ASC, version ASC
LIMIT $2;

-- name: GetLatestVersion :one
SELECT COALESCE(MAX(version), 0)::int AS version
FROM events
WHERE aggregate_type = $1
  AND aggregate_id = $2;
