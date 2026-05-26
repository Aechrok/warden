-- name: GetUserByID :one
SELECT id, email, name, is_active, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, name, is_active, created_at, updated_at
FROM users
WHERE email = $1;

-- name: CreateUser :one
INSERT INTO users (email, name, is_active)
VALUES ($1, $2, COALESCE($3, true))
RETURNING id, email, name, is_active, created_at, updated_at;

-- name: UpdateUser :one
UPDATE users
SET name      = COALESCE($2, name),
    is_active = COALESCE($3, is_active),
    updated_at = now()
WHERE id = $1
RETURNING id, email, name, is_active, created_at, updated_at;

-- name: ListUsers :many
SELECT id, email, name, is_active, created_at, updated_at
FROM users
ORDER BY email ASC
LIMIT $1 OFFSET $2;
