package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
)

type ctxKey int

const (
	ctxUser  ctxKey = iota
	ctxToken ctxKey = iota
)

// TokenClaims holds the resolved identity from an API bearer token.
type TokenClaims struct {
	TokenID uuid.UUID
	UserID  uuid.UUID
	Scopes  []string
}

// UserFromCtx extracts the authenticated *domain.User from the context.
// Returns false if no user was stored (i.e. not a session-authenticated request).
func UserFromCtx(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(ctxUser).(*domain.User)
	return u, ok && u != nil
}

// TokenFromCtx extracts the *TokenClaims from the context.
// Returns false if no token claims were stored (i.e. not a token-authenticated request).
func TokenFromCtx(ctx context.Context) (*TokenClaims, bool) {
	t, ok := ctx.Value(ctxToken).(*TokenClaims)
	return t, ok && t != nil
}

// userIDFromCtx returns the actor user ID regardless of whether the request
// was authenticated via session or bearer token.
func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	if u, ok := UserFromCtx(ctx); ok {
		return u.ID, true
	}
	if t, ok := TokenFromCtx(ctx); ok {
		return t.UserID, true
	}
	return uuid.Nil, false
}
