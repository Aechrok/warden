package middleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/rbac"
)

// RequirePermission gates access to the next handler behind an RBAC check.
// The user ID is read from either the session or the token context key,
// whichever is present. Returns 403 if the user lacks the permission.
func RequirePermission(checker *rbac.Checker, pool *pgxpool.Pool, perm rbac.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := resolveUserID(r)
			if userID == uuid.Nil {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			ok, err := checker.HasPermission(r.Context(), pool, userID, perm)
			if err != nil || !ok {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// resolveUserID returns the user ID from whichever auth context key is set.
func resolveUserID(r *http.Request) uuid.UUID {
	if u, ok := UserFromCtx(r.Context()); ok {
		return u.ID
	}
	if t, ok := TokenClaimsFromCtx(r.Context()); ok {
		return t.UserID
	}
	return uuid.Nil
}
