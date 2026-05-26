// Package googleauth implements the Google service-account JWT bearer flow
// used by the google_workspace and google_vault plugins. It is intentionally
// a thin wrapper over golang.org/x/oauth2/google so the plugins themselves
// stay free of OAuth boilerplate.
package googleauth

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

// TokenSource is the minimal token-source interface used by callers. The
// oauth2 package's *jwt.Config satisfies it via TokenSource.
type TokenSource interface {
	Token() (string, error)
}

// AccessToken obtains an OAuth2 bearer token from a service-account JSON
// key. adminEmail is the impersonated user (domain-wide delegation); scopes
// are the OAuth scopes to grant.
func AccessToken(ctx context.Context, serviceAccountJSON []byte, adminEmail string, scopes []string) (string, error) {
	cfg, err := google.JWTConfigFromJSON(serviceAccountJSON, scopes...)
	if err != nil {
		return "", fmt.Errorf("googleauth: parse service account: %w", err)
	}
	cfg.Subject = adminEmail
	tok, err := cfg.TokenSource(ctx).Token()
	if err != nil {
		return "", fmt.Errorf("googleauth: token: %w", err)
	}
	return tok.AccessToken, nil
}

// NewClient returns an *http.Client whose Authorization header is populated
// automatically from a service-account JWT source. Useful when a plugin
// needs to make many calls in a row without re-fetching tokens.
func NewClient(ctx context.Context, serviceAccountJSON []byte, adminEmail string, scopes []string) (*http.Client, error) {
	cfg, err := google.JWTConfigFromJSON(serviceAccountJSON, scopes...)
	if err != nil {
		return nil, fmt.Errorf("googleauth: parse service account: %w", err)
	}
	cfg.Subject = adminEmail
	return cfg.Client(ctx), nil
}

// ConfigFromJSON returns the raw *jwt.Config in case a caller needs finer
// control (token caching, custom transport, etc.).
func ConfigFromJSON(serviceAccountJSON []byte, adminEmail string, scopes []string) (*jwt.Config, error) {
	cfg, err := google.JWTConfigFromJSON(serviceAccountJSON, scopes...)
	if err != nil {
		return nil, fmt.Errorf("googleauth: parse service account: %w", err)
	}
	cfg.Subject = adminEmail
	return cfg, nil
}
