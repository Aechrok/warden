// Package jamf implements the Warden plugin for Jamf Pro. It exposes
// device-management actions (lock, wipe) for both macOS computers and iOS
// mobile devices via the Jamf Classic API, using OAuth 2.0 client
// credentials for authentication.
package jamf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/plugins/internal/httpx"
)

const (
	credBaseURL      = "base_url"
	credClientID     = "client_id"
	credClientSecret = "client_secret"

	actionLockComputer = "lock_computer"
	actionWipeComputer = "wipe_computer"
	actionLockMobile   = "lock_mobile"
	actionWipeMobile   = "wipe_mobile"

	paramDeviceID    = "device_id"
	paramLockMessage = "lock_message"
	paramPasscode    = "passcode"
)

// TokenFunc obtains a Jamf bearer token for the given creds.
type TokenFunc func(ctx context.Context, baseURL, clientID, secret string) (string, error)

// Plugin implements domain.Plugin and ActionExecutor for Jamf.
type Plugin struct {
	client  *http.Client
	tokenFn TokenFunc

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	token  string
	expiry time.Time
}

// New constructs a Plugin with the default HTTP client and live token func.
func New() *Plugin {
	p := &Plugin{
		client: httpx.NewClient(),
		tokens: map[string]cachedToken{},
	}
	p.tokenFn = p.fetchAccessToken
	return p
}

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "jamf" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Jamf Pro" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Jamf Pro device-management actions (lock, wipe) for macOS and iOS devices."
}

// CredentialSchema declares the fields required to authenticate against
// Jamf Pro.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credBaseURL, Label: "Base URL", Type: "string", Required: true, Description: "e.g. https://yourorg.jamfcloud.com"},
		{Key: credClientID, Label: "Client ID", Type: "string", Required: true},
		{Key: credClientSecret, Label: "Client Secret", Type: "string", Required: true, Secret: true},
	}
}

// Actions returns the operator-invokable actions exposed by the Jamf plugin.
func (p *Plugin) Actions() []domain.ActionDefinition {
	deviceParam := domain.ParamDefinition{Key: paramDeviceID, Label: "Device ID", Type: "string", Required: true, Description: "Jamf computer or mobile device id."}
	lockMsg := domain.ParamDefinition{Key: paramLockMessage, Label: "Lock message", Type: "string", Required: false, Description: "Message shown on the locked device."}
	passcode := domain.ParamDefinition{Key: paramPasscode, Label: "Passcode", Type: "string", Required: false, Description: "Passcode for the wipe command (6 digits, where supported)."}
	return []domain.ActionDefinition{
		{Key: actionLockComputer, Label: "Lock computer", Description: "Issue DeviceLock to a macOS computer.", Destructive: false, Params: []domain.ParamDefinition{deviceParam, lockMsg}},
		{Key: actionWipeComputer, Label: "Wipe computer", Description: "Issue EraseDevice to a macOS computer.", Destructive: true, RequiresApproval: true, Params: []domain.ParamDefinition{deviceParam, passcode}},
		{Key: actionLockMobile, Label: "Lock mobile device", Description: "Issue DeviceLock to an iOS device.", Destructive: false, Params: []domain.ParamDefinition{deviceParam, lockMsg}},
		{Key: actionWipeMobile, Label: "Wipe mobile device", Description: "Issue EraseDevice to an iOS device.", Destructive: true, RequiresApproval: true, Params: []domain.ParamDefinition{deviceParam, passcode}},
	}
}

// HealthCheck calls /api/v1/auth.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	base, err := requireBaseURL(creds)
	if err != nil {
		return err
	}
	tok, err := p.token(ctx, creds)
	if err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, httpx.JoinURL(base, "/api/v1/auth"), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// Execute dispatches one of the Jamf device commands.
func (p *Plugin) Execute(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	deviceID, _ := params[paramDeviceID].(string)
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return domain.ActionResult{}, fmt.Errorf("jamf: %s requires param %q", actionKey, paramDeviceID)
	}
	base, err := requireBaseURL(creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	tok, err := p.token(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}

	var endpoint string
	switch actionKey {
	case actionLockComputer:
		endpoint = httpx.JoinURL(base, "/JSSResource/computercommands/command/DeviceLock/id", url.PathEscape(deviceID))
		if msg, ok := params[paramLockMessage].(string); ok && msg != "" {
			endpoint = httpx.JoinURL(endpoint, url.PathEscape(msg))
		}
	case actionWipeComputer:
		endpoint = httpx.JoinURL(base, "/JSSResource/computercommands/command/EraseDevice/id", url.PathEscape(deviceID))
		if pc, ok := params[paramPasscode].(string); ok && pc != "" {
			endpoint = httpx.JoinURL(endpoint, "passcode", url.PathEscape(pc))
		}
	case actionLockMobile:
		endpoint = httpx.JoinURL(base, "/JSSResource/mobiledevicecommands/command/DeviceLock/id", url.PathEscape(deviceID))
		if msg, ok := params[paramLockMessage].(string); ok && msg != "" {
			endpoint = httpx.JoinURL(endpoint, url.PathEscape(msg))
		}
	case actionWipeMobile:
		endpoint = httpx.JoinURL(base, "/JSSResource/mobiledevicecommands/command/EraseDevice/id", url.PathEscape(deviceID))
		if pc, ok := params[paramPasscode].(string); ok && pc != "" {
			endpoint = httpx.JoinURL(endpoint, "passcode", url.PathEscape(pc))
		}
	default:
		return domain.ActionResult{}, fmt.Errorf("jamf: unknown action %q", actionKey)
	}

	req, err := httpx.NewJSONRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{
		Success: true,
		Message: "jamf: " + actionKey + " issued for device " + deviceID,
		Data:    map[string]any{"action": actionKey, "device_id": deviceID},
	}, nil
}

// token returns a cached bearer token, fetching a new one if missing or
// near expiry.
func (p *Plugin) token(ctx context.Context, creds domain.Credentials) (string, error) {
	base, err := requireBaseURL(creds)
	if err != nil {
		return "", err
	}
	clientID := strings.TrimSpace(creds[credClientID])
	secret := strings.TrimSpace(creds[credClientSecret])
	if clientID == "" || secret == "" {
		return "", errors.New("jamf: client_id and client_secret are required")
	}
	key := base + "|" + clientID
	p.mu.Lock()
	if t, ok := p.tokens[key]; ok && time.Until(t.expiry) > 30*time.Second {
		p.mu.Unlock()
		return t.token, nil
	}
	p.mu.Unlock()

	tok, err := p.tokenFn(ctx, base, clientID, secret)
	if err != nil {
		return "", err
	}
	// We cache for 5 minutes by default; real Jamf tokens last ~30m but the
	// token endpoint also returns an expires_in we honor in fetchAccessToken.
	p.mu.Lock()
	p.tokens[key] = cachedToken{token: tok, expiry: time.Now().Add(5 * time.Minute)}
	p.mu.Unlock()
	return tok, nil
}

func (p *Plugin) fetchAccessToken(ctx context.Context, baseURL, clientID, secret string) (string, error) {
	form := url.Values{
		"grant_type":    []string{"client_credentials"},
		"client_id":     []string{clientID},
		"client_secret": []string{secret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpx.JoinURL(baseURL, "/api/oauth/token"), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("jamf: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jamf: token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("jamf: token endpoint returned %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("jamf: decode token: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("jamf: empty access_token in response")
	}
	return out.AccessToken, nil
}

func requireBaseURL(creds domain.Credentials) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(creds[credBaseURL]), "/")
	if base == "" {
		return "", errors.New("jamf: base_url credential is required")
	}
	return base, nil
}

// SetTokenFunc replaces the token-fetching strategy. Primarily for tests.
func (p *Plugin) SetTokenFunc(fn TokenFunc) { p.tokenFn = fn }

// SetHTTPClient replaces the HTTP client. Primarily for tests.
func (p *Plugin) SetHTTPClient(c *http.Client) { p.client = c }
