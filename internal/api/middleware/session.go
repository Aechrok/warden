// Package middleware contains reusable HTTP middleware for the Warden API.
package middleware

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/domain"
)

type sessionCtxKey struct{}
type userCtxKey struct{}

// SessionAuth reads the warden_session cookie, validates it, and stores the
// *domain.User in the request context. Requests with invalid or missing
// sessions receive a 401.
func SessionAuth(sessions *auth.SessionStore, users *auth.UserStore, pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("warden_session")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			session, err := sessions.Validate(r.Context(), pool, cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			user, err := users.GetByID(r.Context(), pool, session.UserID)
			if err != nil || !user.IsActive {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userCtxKey{}, user)
			ctx = context.WithValue(ctx, sessionCtxKey{}, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromCtx extracts the *domain.User stored by SessionAuth.
func UserFromCtx(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(*domain.User)
	return u, ok && u != nil
}

// SessionFromCtx extracts the *auth.SessionData stored by SessionAuth.
func SessionFromCtx(ctx context.Context) (*auth.SessionData, bool) {
	s, ok := ctx.Value(sessionCtxKey{}).(*auth.SessionData)
	return s, ok && s != nil
}
