package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookieName = "warden_session"
const sessionTTL = 8 * time.Hour

// AuthHandler handles the OIDC login / callback flow.
type AuthHandler struct {
	provider *auth.Provider
	sessions *auth.SessionStore
	users    *auth.UserStore
	pool     *pgxpool.Pool
	logger   *zap.Logger
	secure   bool
}

// NewAuthHandler constructs the OIDC auth handler.
func NewAuthHandler(provider *auth.Provider, sessions *auth.SessionStore, users *auth.UserStore, pool *pgxpool.Pool, logger *zap.Logger, secure bool) *AuthHandler {
	return &AuthHandler{
		provider: provider,
		sessions: sessions,
		users:    users,
		pool:     pool,
		logger:   logger,
		secure:   secure,
	}
}

// Login redirects to the OIDC authorization endpoint.
// POST /api/v1/internal/auth/login  (no auth required)
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	authURL := h.provider.AuthURL("warden-state", "")
	if authURL == "" {
		http.Error(w, `{"error":"OIDC not configured"}`, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback exchanges the OIDC code, upserts the user, creates a session,
// sets the session cookie, and redirects to /.
// GET /auth/callback  (no auth required)
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, `{"error":"missing code"}`, http.StatusBadRequest)
		return
	}

	tokens, err := h.provider.Exchange(r.Context(), code, "")
	if err != nil {
		h.logger.Error("auth: OIDC exchange failed", zap.Error(err))
		http.Error(w, `{"error":"authentication failed"}`, http.StatusUnauthorized)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	user, err := h.users.GetOrCreate(r.Context(), tx, tokens.Email, tokens.Name)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	sessionToken, err := h.sessions.Create(r.Context(), tx, user.ID, sessionTTL)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// generateAPIToken returns a cryptographically random raw token and its SHA-256 hex hash.
func generateAPIToken() (raw, hash string) {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return
}
