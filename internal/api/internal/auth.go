package internal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/rbac"
)

const sessionCookieName = "warden_session"
const oidcStateCookieName = "oidc_state"
const sessionTTL = 8 * time.Hour

// AuthHandler handles login flows: OIDC, local password, and magic-link redemption.
type AuthHandler struct {
	provider *auth.Provider
	sessions *auth.SessionStore
	users    *auth.UserStore
	pool     *pgxpool.Pool
	logger   *zap.Logger
	secure   bool
}

// NewAuthHandler constructs the auth handler. provider may be nil if OIDC is not configured.
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

// GetAuthConfig returns the public authentication configuration for the login page.
// GET /auth/config  (no auth required)
func (h *AuthHandler) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	var ssoEnabled, enforceSSO bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT sso_enabled, enforce_sso FROM sso_config WHERE singleton = true
	`).Scan(&ssoEnabled, &enforceSSO)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"sso_enabled": false, "enforce_sso": false})
		return
	}
	effectiveSSO := ssoEnabled && h.provider != nil
	writeJSON(w, http.StatusOK, map[string]any{
		"sso_enabled": effectiveSSO,
		"enforce_sso": enforceSSO && effectiveSSO,
	})
}

// Login redirects to the OIDC authorization endpoint with a random per-request CSRF state.
// POST /api/v1/internal/auth/login  (no auth required)
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		http.Error(w, `{"error":"SSO not configured"}`, http.StatusNotImplemented)
		return
	}

	// Generate a cryptographically random CSRF state nonce.
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(buf)

	// Store the nonce in a short-lived HTTP-only cookie so Callback can verify it.
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((5 * time.Minute).Seconds()),
	})

	authURL := h.provider.AuthURL(state, "")
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
	// Verify the CSRF state nonce before processing the code.
	returnedState := r.URL.Query().Get("state")
	stateCookie, cookieErr := r.Cookie(oidcStateCookieName)
	// Clear the state cookie regardless of outcome to prevent replay.
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	if cookieErr != nil || returnedState == "" || stateCookie.Value != returnedState {
		h.logger.Warn("auth: OIDC state mismatch", zap.String("returned", returnedState))
		http.Error(w, `{"error":"invalid state"}`, http.StatusBadRequest)
		return
	}

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

	// Track that this user most recently authenticated via OIDC.
	if _, err := tx.Exec(r.Context(), `UPDATE users SET origin = 'oidc' WHERE id = $1`, user.ID); err != nil {
		h.logger.Warn("auth: could not update origin", zap.Error(err))
	}

	if err := rbac.BootstrapFirstAdmin(r.Context(), tx, user.ID); err != nil {
		h.logger.Warn("auth: bootstrap admin check failed", zap.Error(err))
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

	h.setSessionCookie(w, sessionToken)
	http.Redirect(w, r, "/", http.StatusFound)
}

// LocalLogin authenticates with email + password.
// POST /auth/local  (no auth required)
func (h *AuthHandler) LocalLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password required")
		return
	}

	// Reject password login when SSO is configured and enforced.
	var ssoEnabled, enforceSSO bool
	if err := h.pool.QueryRow(r.Context(), `
		SELECT sso_enabled, enforce_sso FROM sso_config WHERE singleton = true
	`).Scan(&ssoEnabled, &enforceSSO); err == nil && enforceSSO && ssoEnabled && h.provider != nil {
		writeError(w, http.StatusForbidden, "SSO login is required")
		return
	}

	var userID uuid.UUID
	var isActive bool
	var passwordHash *string
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, is_active, password_hash FROM users WHERE email = $1
	`, email).Scan(&userID, &isActive, &passwordHash)

	// Surface real DB errors as 500 before applying auth logic.
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.logger.Error("local login: db error", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || passwordHash == nil || *passwordHash == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Verify password before revealing whether the account is active, to
	// prevent confirming account existence via the "account inactive" response.
	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !isActive {
		writeError(w, http.StatusUnauthorized, "account inactive")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := rbac.BootstrapFirstAdmin(r.Context(), tx, userID); err != nil {
		h.logger.Warn("local login: bootstrap admin check failed", zap.Error(err))
	}

	sessionToken, err := h.sessions.Create(r.Context(), tx, userID, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.setSessionCookie(w, sessionToken)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RedeemMagicLink validates a magic link token, creates a session, and redirects to /.
// GET /auth/magic?token=...  (no auth required)
func (h *AuthHandler) RedeemMagicLink(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	var linkID uuid.UUID
	var email string
	var roleName *string
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, email, role_name FROM magic_links
		WHERE token = $1 AND used_at IS NULL AND expires_at > now()
	`, token).Scan(&linkID, &email, &roleName)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "link invalid or expired", http.StatusUnauthorized)
		return
	}
	if err != nil {
		h.logger.Error("magic link: db error", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	user, err := h.users.GetOrCreate(r.Context(), tx, email, "")
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if roleName != nil && *roleName != "" {
		var roleID uuid.UUID
		if err2 := tx.QueryRow(r.Context(), `SELECT id FROM roles WHERE name = $1`, *roleName).Scan(&roleID); err2 == nil {
			_, _ = tx.Exec(r.Context(), `
				INSERT INTO user_roles (user_id, role_id, granted_by)
				VALUES ($1, $2, $1) ON CONFLICT DO NOTHING
			`, user.ID, roleID)
		}
	}

	// Mark the link used inside the transaction so that if the commit fails
	// the link remains valid and the session is never issued.
	if _, err := tx.Exec(r.Context(), `UPDATE magic_links SET used_at = now() WHERE id = $1`, linkID); err != nil {
		h.logger.Error("magic link: mark used", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := rbac.BootstrapFirstAdmin(r.Context(), tx, user.ID); err != nil {
		h.logger.Warn("magic link: bootstrap admin check failed", zap.Error(err))
	}

	sessionToken, err := h.sessions.Create(r.Context(), tx, user.ID, sessionTTL)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.setSessionCookie(w, sessionToken)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
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

// HashPassword bcrypt-hashes a plaintext password. Returns an error if the
// password is shorter than 8 characters or longer than 72 bytes (bcrypt's
// silent truncation boundary).
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	if len(password) > 72 {
		return "", errors.New("password must not exceed 72 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
