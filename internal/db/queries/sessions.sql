-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, token_hash, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT id, user_id, token_hash, expires_at, created_at
FROM sessions
WHERE token_hash = $1
  AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= now();
