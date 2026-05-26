-- name: CreateInstance :one
INSERT INTO integration_instances (name, plugin_id, credentials_enc, is_active)
VALUES ($1, $2, $3, COALESCE($4, true))
RETURNING id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at;

-- name: GetInstanceByID :one
SELECT id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at
FROM integration_instances
WHERE id = $1;

-- name: GetInstanceByName :one
SELECT id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at
FROM integration_instances
WHERE name = $1;

-- name: ListInstances :many
SELECT id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at
FROM integration_instances
ORDER BY name ASC;

-- name: UpdateInstance :one
UPDATE integration_instances
SET name            = COALESCE($2, name),
    plugin_id       = COALESCE($3, plugin_id),
    credentials_enc = COALESCE($4, credentials_enc),
    is_active       = COALESCE($5, is_active),
    updated_at      = now()
WHERE id = $1
RETURNING id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at;

-- name: UpdateInstanceHealth :one
UPDATE integration_instances
SET last_health_ok = $2,
    last_health_at = now(),
    updated_at     = now()
WHERE id = $1
RETURNING id, name, plugin_id, credentials_enc, is_active, last_health_ok, last_health_at, created_at, updated_at;
