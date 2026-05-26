package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionTokenBytes is the length of the random session token in bytes
// before base64 encoding. 32 bytes is comfortably above the recommended
// 128-bit entropy floor for opaque session tokens.
const SessionTokenBytes = 32

// ErrSessionNotFound is returned by Validate when the token does not match
// any active (non-expired) session.
var ErrSessionNotFound = errors.New("auth: session not found or expired")

// SessionData is the small subset of session columns that callers need to
// authorize a request. The raw token is never persisted; only its SHA-256
// digest, hex-encoded.
type SessionData struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	CreatedAt time.Time
}

// SessionStore is a thin wrapper over the `sessions` table. The queries are
// simple enough that raw pgx is preferable to sqlc-generated code: callers
// keep direct control over the transaction boundary used by Create.
type SessionStore struct{}

// NewSessionStore constructs a SessionStore. There is no per-instance state;
// the value exists purely to give the methods a namespace.
func NewSessionStore() *SessionStore {
	return &SessionStore{}
}

// Create generates a fresh session token, stores its SHA-256 hash under the
// supplied userID, and returns the raw token (which is sent to the client
// as a cookie). ttl controls how long the session is valid.
func (s *SessionStore) Create(ctx context.Context, tx pgx.Tx, userID uuid.UUID, ttl time.Duration) (string, error) {
	if userID == uuid.Nil {
		return "", errors.New("auth: session create: nil user id")
	}
	if ttl <= 0 {
		return "", errors.New("auth: session create: ttl must be positive")
	}

	token, err := generateToken(SessionTokenBytes)
	if err != nil {
		return "", fmt.Errorf("auth: generate session token: %w", err)
	}
	hash := hashToken(token)
	expiresAt := time.Now().Add(ttl)

	if _, err := tx.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, hash, expiresAt); err != nil {
		return "", fmt.Errorf("auth: insert session: %w", err)
	}

	return token, nil
}

// Validate looks up the session by token hash and returns its data if the
// session exists and has not yet expired. The lookup runs on the pool
// directly (no caller-managed tx); reads do not need transaction isolation.
func (s *SessionStore) Validate(ctx context.Context, pool *pgxpool.Pool, token string) (*SessionData, error) {
	if pool == nil {
		return nil, errors.New("auth: session validate: nil pool")
	}
	if token == "" {
		return nil, ErrSessionNotFound
	}
	hash := hashToken(token)

	var data SessionData
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at, created_at
		FROM sessions
		WHERE token_hash = $1
		  AND expires_at > now()
	`, hash).Scan(&data.ID, &data.UserID, &data.ExpiresAt, &data.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("auth: select session: %w", err)
	}
	return &data, nil
}

// Delete revokes a session by raw token. Idempotent: deleting a token that
// does not exist returns nil.
func (s *SessionStore) Delete(ctx context.Context, pool *pgxpool.Pool, token string) error {
	if pool == nil {
		return errors.New("auth: session delete: nil pool")
	}
	if token == "" {
		return nil
	}
	hash := hashToken(token)
	if _, err := pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// DeleteByID revokes a single session by primary key. Useful for the "log
// out everywhere except this session" admin flow.
func (s *SessionStore) DeleteByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	if pool == nil {
		return errors.New("auth: session delete: nil pool")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("auth: delete session by id: %w", err)
	}
	return nil
}

// DeleteAllForUser revokes every active session for a given operator.
func (s *SessionStore) DeleteAllForUser(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) error {
	if pool == nil {
		return errors.New("auth: session delete: nil pool")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("auth: delete sessions by user: %w", err)
	}
	return nil
}

// CountActiveForUser returns the number of active (non-expired) sessions
// for the given operator. Used by the concurrent_session_limit policy.
func (s *SessionStore) CountActiveForUser(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) (int, error) {
	if pool == nil {
		return 0, errors.New("auth: session count: nil pool")
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM sessions
		WHERE user_id = $1
		  AND expires_at > now()
	`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("auth: count sessions: %w", err)
	}
	return n, nil
}

// PurgeExpired deletes all sessions whose expires_at has passed. Intended
// to be called by a periodic River job; safe to invoke ad-hoc.
func (s *SessionStore) PurgeExpired(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if pool == nil {
		return 0, errors.New("auth: session purge: nil pool")
	}
	tag, err := pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// generateToken returns a URL-safe random token of the requested raw byte
// length. The result is base64 (URL, no padding) and has roughly
// ceil(n*8/6) characters.
func generateToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashToken returns the lowercase hex SHA-256 digest of the token. We store
// the hash, not the raw token, so a database read leak does not directly
// expose live sessions.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
