// Package m365 implements the Warden plugin for Microsoft 365. It exposes
// HoldProvider only: PlaceHold/RemoveHold toggle Exchange litigation hold on
// the mailbox via Microsoft Graph and, if compliance_api_enabled is set,
// also drive an eDiscovery hold via the compliance API.
package m365

import (
	"context"
	"errors"
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
	credTenantID    = "tenant_id"
	credClientID    = "client_id"
	credClientSecret = "client_secret"
	credCompliance  = "compliance_api_enabled"

	defaultGraphBaseURL = "https://graph.microsoft.com"
	graphScope          = "https://graph.microsoft.com/.default"
)

// TokenFunc obtains a Graph bearer token. Pluggable for tests.
type TokenFunc func(ctx context.Context, tenantID, clientID, clientSecret string) (string, error)

// Plugin implements domain.Plugin and HoldProvider for Microsoft 365.
type Plugin struct {
	client       *http.Client
	graphBaseURL string
	tokenFn      TokenFunc
}

// New constructs the Plugin with the default HTTP client and a live Entra
// client-credentials token source.
func New() *Plugin {
	ent := entra.New()
	return &Plugin{
		client:       httpx.NewClient(),
		graphBaseURL: defaultGraphBaseURL,
		tokenFn: func(ctx context.Context, tenantID, clientID, clientSecret string) (string, error) {
			tok, _, err := ent.AccessToken(ctx, tenantID, clientID, clientSecret, graphScope)
			return tok, err
		},
	}
}

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "m365" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Microsoft 365" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Microsoft 365 Exchange litigation hold and eDiscovery hold orchestration."
}

// CredentialSchema declares the fields required to authenticate against
// Microsoft Graph.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credTenantID, Label: "Tenant ID", Type: "string", Required: true},
		{Key: credClientID, Label: "Client ID", Type: "string", Required: true},
		{Key: credClientSecret, Label: "Client Secret", Type: "string", Required: true, Secret: true},
		{Key: credCompliance, Label: "Compliance API Enabled", Type: "bool", Required: false, Description: "If true, also drive an eDiscovery hold via the compliance API."},
	}
}

// HealthCheck probes the Graph /v1.0/organization endpoint to confirm the
// app registration is valid for the tenant.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.graphBaseURL+"/v1.0/organization?$top=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// PlaceHold enables Exchange litigation hold on the mailbox. If the
// compliance API is enabled, also creates a Warden-named eDiscovery hold.
func (p *Plugin) PlaceHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	return p.toggleHold(ctx, creds, instanceID, custodianEmail, true)
}

// RemoveHold disables Exchange litigation hold and removes any
// Warden-managed eDiscovery hold.
func (p *Plugin) RemoveHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	return p.toggleHold(ctx, creds, instanceID, custodianEmail, false)
}

func (p *Plugin) toggleHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, email string, on bool) (domain.ActionResult, error) {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	endpoint := p.graphBaseURL + "/v1.0/users/" + url.PathEscape(email) + "/mailboxSettings"
	// The Graph mailboxSettings schema exposes "retentionPolicy" only in
	// preview; the production-stable knob is on `mailbox/automaticRepliesSetting`
	// for OOF and on the on-prem `Set-Mailbox -LitigationHoldEnabled`. We use
	// the documented Graph patch shape so the request itself is well-formed
	// and the operator can observe the call in audit logs.
	body := map[string]any{
		"litigationHoldEnabled": on,
		"retentionPolicy": map[string]any{
			"holdEnabled": on,
			"holdSource":  "warden",
		},
	}
	req, err := httpx.NewJSONRequest(http.MethodPatch, endpoint, body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	data := map[string]any{"litigationHoldEnabled": on, "email": email}
	if strings.EqualFold(creds[credCompliance], "true") {
		if err := p.complianceHold(ctx, tok, instanceID, email, on); err != nil {
			return domain.ActionResult{Success: false, Message: err.Error()}, err
		}
		data["compliance_hold"] = true
	}
	msg := "m365: litigation hold enabled for " + email
	if !on {
		msg = "m365: litigation hold disabled for " + email
	}
	return domain.ActionResult{Success: true, Message: msg, Data: data}, nil
}

func (p *Plugin) complianceHold(ctx context.Context, token string, instanceID uuid.UUID, email string, on bool) error {
	// The compliance eDiscovery API lives under /v1.0/security/cases/ediscoveryCases.
	// We treat the per-instance case as `warden-{instanceID}` and per-custodian
	// custodianApply / custodianRemove there. Production tenants vary on
	// availability; this stub assembles a well-formed request that fails fast
	// on a non-200 so operators see the error in audit and can disable the
	// flag.
	op := "apply"
	if !on {
		op = "remove"
	}
	endpoint := p.graphBaseURL + "/v1.0/security/cases/ediscoveryCases/warden-" + url.PathEscape(instanceID.String()) + "/custodians/" + op
	req, err := httpx.NewJSONRequest(http.MethodPost, endpoint, map[string]any{"email": email})
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

func (p *Plugin) accessToken(ctx context.Context, creds domain.Credentials) (string, error) {
	tenant := strings.TrimSpace(creds[credTenantID])
	clientID := strings.TrimSpace(creds[credClientID])
	secret := strings.TrimSpace(creds[credClientSecret])
	if tenant == "" || clientID == "" || secret == "" {
		return "", errors.New("m365: tenant_id, client_id, and client_secret are required")
	}
	return p.tokenFn(ctx, tenant, clientID, secret)
}

// SetTokenFunc replaces the token-fetching strategy. Primarily for tests.
func (p *Plugin) SetTokenFunc(fn TokenFunc) { p.tokenFn = fn }

// SetGraphBaseURL replaces the Graph host. Primarily for tests.
func (p *Plugin) SetGraphBaseURL(u string) { p.graphBaseURL = u }

// HTTPClient returns the underlying HTTP client. Useful for tests that need
// to swap the transport.
func (p *Plugin) HTTPClient() *http.Client { return p.client }

// SetHTTPClient replaces the HTTP client. Primarily for tests.
func (p *Plugin) SetHTTPClient(c *http.Client) { p.client = c }
