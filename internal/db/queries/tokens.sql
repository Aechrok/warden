-- name: CreateToken :one
INSERT INTO api_tokens (user_id, name, token_hash, scopes, expires_at)
VALUES ($1, $2, $3, COALESCE($4, '{}'::text[]), $5)
RETURNING id, user_id, name, token_hash, scopes, last_used, expires_at, created_at;

-- name: GetTokenByHash :one
SELECT id, user_id, name, token_hash, scopes, last_used, expires_at, created_at
FROM api_tokens
WHERE token_hash = $1
  AND (expires_at IS NULL OR expires_at > now());

-- name: ListTokensByUser :many
SELECT id, user_id, name, token_hash, scopes, last_used, expires_at, created_at
FROM api_tokens
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: DeleteToken :exec
DELETE FROM api_tokens
WHERE id = $1
  AND user_id = $2;
