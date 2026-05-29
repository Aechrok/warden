// Package auth implements Warden's identity-layer primitives: a generic
// OIDC client, the session store backing operator login, and operator
// account upsert. No IdP-specific code lives here — any compliant OIDC
// provider can be wired in via configuration.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/aechrok/warden/internal/config"
)

// DefaultScopes is the OIDC scope set requested at authorization time. The
// operator's email and name are required to provision the Warden user record.
var DefaultScopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}

// Tokens is the result of a successful authorization-code exchange. Email,
// Name, and Subject come from the validated ID token claims.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Subject      string
	Email        string
	Name         string
}

// Provider wraps the zitadel/oidc relying-party client. It is safe for
// concurrent use; the underlying discovery is performed once at construction.
type Provider struct {
	rp          rp.RelyingParty
	redirectURL string
}

// NewProvider discovers the OIDC issuer's metadata and constructs a relying
// party. Returns nil, nil when OIDC is not configured — callers must handle a
// nil provider (local-password auth still works without it).
func NewProvider(ctx context.Context, cfg *config.Config) (*Provider, error) {
	if cfg == nil {
		return nil, errors.New("auth: nil config")
	}
	if strings.TrimSpace(cfg.OIDCIssuer) == "" || strings.TrimSpace(cfg.OIDCClientID) == "" {
		return nil, nil
	}

	redirectURL := strings.TrimSpace(cfg.OIDCRedirectURL)
	if redirectURL == "" {
		redirectURL = fmt.Sprintf("http://localhost:%d/auth/callback", cfg.ServerPort)
	}

	httpClient := oidcHTTPClient(cfg)
	options := []rp.Option{
		rp.WithHTTPClient(httpClient),
	}

	provider, err := rp.NewRelyingPartyOIDC(
		ctx,
		cfg.OIDCIssuer,
		cfg.OIDCClientID,
		cfg.OIDCSecret,
		redirectURL,
		DefaultScopes,
		options...,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: discover OIDC issuer: %w", err)
	}

	return &Provider{rp: provider, redirectURL: redirectURL}, nil
}

// AuthURL constructs the authorization endpoint URL the browser should be
// redirected to. The caller supplies an opaque state value (CSRF) and a
// nonce (replay protection); both are echoed back via the callback.
func (p *Provider) AuthURL(state, nonce string) string {
	if p == nil || p.rp == nil {
		return ""
	}
	var authOpts []rp.AuthURLOpt
	if nonce != "" {
		param := rp.WithURLParam("nonce", nonce)
		authOpts = append(authOpts, rp.AuthURLOpt(param))
	}
	return rp.AuthURL(state, p.rp, authOpts...)
}

// RedirectURL returns the configured callback URL.
func (p *Provider) RedirectURL() string {
	if p == nil {
		return ""
	}
	return p.redirectURL
}

// Exchange completes the authorization-code exchange and validates the
// returned ID token. The supplied codeVerifier must be the PKCE verifier
// generated alongside the code_challenge sent on AuthURL.
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier string) (*Tokens, error) {
	if p == nil || p.rp == nil {
		return nil, errors.New("auth: provider not initialized")
	}
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("auth: empty authorization code")
	}

	opts := []rp.CodeExchangeOpt{}
	if codeVerifier != "" {
		opts = append(opts, rp.WithCodeVerifier(codeVerifier))
	}

	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, p.rp, opts...)
	if err != nil {
		return nil, fmt.Errorf("auth: code exchange: %w", err)
	}
	if tokens.IDTokenClaims == nil {
		return nil, errors.New("auth: missing ID token claims")
	}

	claims := tokens.IDTokenClaims
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return nil, errors.New("auth: ID token missing email claim")
	}

	name := strings.TrimSpace(claims.Name)
	if name == "" {
		// Fall back to the preferred username, then to the local-part of email.
		if pu := claims.PreferredUsername; pu != "" {
			name = pu
		} else if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		} else {
			name = email
		}
	}

	subject := strings.TrimSpace(claims.Subject)
	if subject == "" {
		return nil, errors.New("auth: ID token missing subject claim")
	}

	out := &Tokens{
		AccessToken: tokens.AccessToken,
		IDToken:     tokens.IDToken,
		Subject:     subject,
		Email:       strings.ToLower(email),
		Name:        name,
	}
	if tokens.RefreshToken != "" {
		out.RefreshToken = tokens.RefreshToken
	}
	return out, nil
}

// ValidateState is a small helper that compares two states with constant-time
// semantics suitable for short opaque tokens. Callers should still bind the
// state cookie to the session out-of-band.
func ValidateState(expected, actual string) bool {
	if expected == "" || actual == "" {
		return false
	}
	if len(expected) != len(actual) {
		return false
	}
	var diff byte
	for i := 0; i < len(expected); i++ {
		diff |= expected[i] ^ actual[i]
	}
	return diff == 0
}

// oidcHTTPClient returns an HTTP client suitable for OIDC discovery and token
// exchange. When OIDCInternalIssuer is set, outbound connections to the public
// issuer host are dialed to the internal host instead — the Host header is
// preserved so the IdP's own issuer-match check still passes. This lets the
// browser use a host it can reach (e.g. localhost) while the server connects
// to a Docker-internal service name (e.g. dex).
func oidcHTTPClient(cfg *config.Config) *http.Client {
	if cfg.OIDCInternalIssuer == "" {
		return http.DefaultClient
	}
	pubURL, err := url.Parse(cfg.OIDCIssuer)
	if err != nil {
		return http.DefaultClient
	}
	intURL, err := url.Parse(cfg.OIDCInternalIssuer)
	if err != nil {
		return http.DefaultClient
	}
	pubHost := pubURL.Host
	intHost := intURL.Host
	if pubHost == "" || intHost == "" {
		return http.DefaultClient
	}
	base := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr == pubHost {
				addr = intHost
			}
			return base.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{Transport: transport}
}

// IssuerHost returns the host of the configured OIDC issuer, useful for log
// fields and admin UI display. Empty string if the issuer is unparseable.
func IssuerHost(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil {
		return ""
	}
	return u.Host
}
