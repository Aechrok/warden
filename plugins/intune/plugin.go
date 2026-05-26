// Package intune implements the Warden plugin for Microsoft Intune. It
// exposes device-management actions (wipe, lock, retire) against managed
// devices via Microsoft Graph.
package intune

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/plugins/internal/entra"
	"github.com/aechrok/warden/plugins/internal/httpx"
)

const (
	credTenantID     = "tenant_id"
	credClientID     = "client_id"
	credClientSecret = "client_secret"

	actionWipe   = "wipe_device"
	actionLock   = "lock_device"
	actionRetire = "retire_device"

	paramDeviceID = "device_id"

	defaultGraphBaseURL = "https://graph.microsoft.com"
	graphScope          = "https://graph.microsoft.com/.default"
)

// TokenFunc obtains a Graph bearer token. Pluggable for tests.
type TokenFunc func(ctx context.Context, tenantID, clientID, clientSecret string) (string, error)

// Plugin implements domain.Plugin and ActionExecutor for Intune.
type Plugin struct {
	client       *http.Client
	graphBaseURL string
	tokenFn      TokenFunc
}

// New constructs the Plugin with the default HTTP client and a live Entra
// token source.
func New() *Plugin {
	ent := entra.New()
	return &Plugin{
		client:       httpx.NewClient(),
		graphBaseURL: defaultGraphBaseURL,
		tokenFn: func(ctx context.Context, tenantID, clientID, secret string) (string, error) {
			tok, _, err := ent.AccessToken(ctx, tenantID, clientID, secret, graphScope)
			return tok, err
		},
	}
}

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "intune" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Microsoft Intune" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Microsoft Intune device-management actions (wipe, lock, retire)."
}

// CredentialSchema declares the fields required to authenticate against
// Microsoft Graph.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credTenantID, Label: "Tenant ID", Type: "string", Required: true},
		{Key: credClientID, Label: "Client ID", Type: "string", Required: true},
		{Key: credClientSecret, Label: "Client Secret", Type: "string", Required: true, Secret: true},
	}
}

// Actions returns the operator-invokable actions exposed by the Intune
// plugin.
func (p *Plugin) Actions() []domain.ActionDefinition {
	deviceParam := domain.ParamDefinition{Key: paramDeviceID, Label: "Device ID", Type: "string", Required: true, Description: "Intune managedDeviceId."}
	return []domain.ActionDefinition{
		{Key: actionWipe, Label: "Wipe device", Description: "Issue a remote wipe to the device.", Destructive: true, RequiresApproval: true, Params: []domain.ParamDefinition{deviceParam}},
		{Key: actionLock, Label: "Lock device", Description: "Remotely lock the device.", Destructive: false, Params: []domain.ParamDefinition{deviceParam}},
		{Key: actionRetire, Label: "Retire device", Description: "Retire the device (removes corporate data, keeps personal data).", Destructive: true, RequiresApproval: true, Params: []domain.ParamDefinition{deviceParam}},
	}
}

// HealthCheck probes the managedDevices endpoint with a single-row page.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.graphBaseURL+"/v1.0/deviceManagement/managedDevices?$top=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// Execute dispatches one of the Intune device actions.
func (p *Plugin) Execute(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	deviceID, _ := params[paramDeviceID].(string)
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return domain.ActionResult{}, fmt.Errorf("intune: %s requires param %q", actionKey, paramDeviceID)
	}
	var op string
	switch actionKey {
	case actionWipe:
		op = "wipe"
	case actionLock:
		op = "remoteLock"
	case actionRetire:
		op = "retire"
	default:
		return domain.ActionResult{}, fmt.Errorf("intune: unknown action %q", actionKey)
	}
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	endpoint := p.graphBaseURL + "/v1.0/deviceManagement/managedDevices/" + url.PathEscape(deviceID) + "/" + op
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
		Message: "intune: " + op + " issued for device " + deviceID,
		Data:    map[string]any{"action": op, "device_id": deviceID},
	}, nil
}

func (p *Plugin) accessToken(ctx context.Context, creds domain.Credentials) (string, error) {
	tenant := strings.TrimSpace(creds[credTenantID])
	clientID := strings.TrimSpace(creds[credClientID])
	secret := strings.TrimSpace(creds[credClientSecret])
	if tenant == "" || clientID == "" || secret == "" {
		return "", errors.New("intune: tenant_id, client_id, and client_secret are required")
	}
	return p.tokenFn(ctx, tenant, clientID, secret)
}

// SetTokenFunc replaces the token-fetching strategy. Primarily for tests.
func (p *Plugin) SetTokenFunc(fn TokenFunc) { p.tokenFn = fn }

// SetGraphBaseURL replaces the Graph host. Primarily for tests.
func (p *Plugin) SetGraphBaseURL(u string) { p.graphBaseURL = u }

// SetHTTPClient replaces the HTTP client. Primarily for tests.
func (p *Plugin) SetHTTPClient(c *http.Client) { p.client = c }
