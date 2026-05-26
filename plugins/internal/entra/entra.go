// Package entra implements the client-credentials flow for Microsoft Entra
// (Azure AD), used by the m365 and intune plugins to obtain Graph API
// bearer tokens.
package entra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aechrok/warden/plugins/internal/httpx"
)

// DefaultLoginHost is the OAuth login endpoint host. Override in tests.
const DefaultLoginHost = "https://login.microsoftonline.com"

// Client is a small helper around the client-credentials flow.
type Client struct {
	HTTPClient *http.Client
	LoginHost  string
}

// New constructs a Client with sensible defaults.
func New() *Client {
	return &Client{HTTPClient: httpx.NewClient(), LoginHost: DefaultLoginHost}
}

// AccessToken fetches an OAuth 2.0 access token for the given tenant +
// client credentials, scoped to the given scope (e.g. "https://graph.microsoft.com/.default").
func (c *Client) AccessToken(ctx context.Context, tenantID, clientID, clientSecret, scope string) (string, time.Time, error) {
	if tenantID == "" || clientID == "" || clientSecret == "" {
		return "", time.Time{}, errors.New("entra: tenant_id, client_id, and client_secret are required")
	}
	form := url.Values{
		"client_id":     []string{clientID},
		"client_secret": []string{clientSecret},
		"scope":         []string{scope},
		"grant_type":    []string{"client_credentials"},
	}
	endpoint := strings.TrimRight(c.LoginHost, "/") + "/" + url.PathEscape(tenantID) + "/oauth2/v2.0/token"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("entra: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req = req.WithContext(ctx)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("entra: do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("entra: token endpoint returned %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", time.Time{}, fmt.Errorf("entra: decode token: %w", err)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("entra: empty access_token (%s: %s)", out.Error, out.ErrorDesc)
	}
	expiry := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return out.AccessToken, expiry, nil
}
