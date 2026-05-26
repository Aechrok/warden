package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/auth"
)

type tokenCtxKey struct{}

// TokenClaims holds the identity resolved from a bearer token.
type TokenClaims struct {
	TokenID uuid.UUID
	UserID  uuid.UUID
	Scopes  []string
}

// TokenAuth reads the Authorization: Bearer <token> header, validates the
// token against the api_tokens table, verifies the token owner is an active
// user, touches last_used, and stores *TokenClaims in the context. Returns
// 401 if invalid, expired, or the owner is inactive.
func TokenAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	users := auth.NewUserStore()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if !strings.HasPrefix(raw, "Bearer ") {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(raw, "Bearer ")
			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			hash := hashAPIToken(token)
			var (
				tokenID   uuid.UUID
				userID    uuid.UUID
				scopes    []string
				expiresAt *time.Time
			)
			err := pool.QueryRow(r.Context(), `
				SELECT id, user_id, scopes, expires_at
				FROM api_tokens
				WHERE token_hash = $1
			`, hash).Scan(&tokenID, &userID, &scopes, &expiresAt)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if expiresAt != nil && time.Now().After(*expiresAt) {
				http.Error(w, `{"error":"token expired"}`, http.StatusUnauthorized)
				return
			}

			// Security: verify the token owner's account is still active.
			// A deactivated user must not be able to use a previously-issued token.
			u, err := users.GetByID(r.Context(), pool, userID)
			if err != nil || !u.IsActive {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Touch last_used asynchronously; do not block the request on it.
			go func() {
				_, _ = pool.Exec(context.Background(), `
					UPDATE api_tokens SET last_used = now() WHERE id = $1
				`, tokenID)
			}()

			claims := &TokenClaims{
				TokenID: tokenID,
				UserID:  userID,
				Scopes:  scopes,
			}
			ctx := context.WithValue(r.Context(), tokenCtxKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TokenClaimsFromCtx extracts the *TokenClaims stored by TokenAuth.
func TokenClaimsFromCtx(ctx context.Context) (*TokenClaims, bool) {
	c, ok := ctx.Value(tokenCtxKey{}).(*TokenClaims)
	return c, ok && c != nil
}

// HasScope returns true if the token claims include the given scope.
func HasScope(ctx context.Context, scope string) bool {
	claims, ok := TokenClaimsFromCtx(ctx)
	if !ok {
		return false
	}
	for _, s := range claims.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func hashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
