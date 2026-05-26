// Package okta implements the Warden plugin for Okta. It exposes user
// lifecycle actions (deactivate, activate, suspend) and an identity lookup
// against the Okta API v1.
package okta

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
	"github.com/aechrok/warden/plugins/internal/httpx"
)

const (
	credBaseURL  = "base_url"
	credAPIToken = "api_token"

	actionDeactivate = "deactivate_user"
	actionActivate   = "activate_user"
	actionSuspend    = "set_blocked_access"
)

// Plugin is the Okta implementation of domain.Plugin, ActionExecutor, and
// IdentityProvider.
type Plugin struct {
	client *http.Client
}

// New constructs a Plugin with the default HTTP client.
func New() *Plugin { return &Plugin{client: httpx.NewClient()} }

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "okta" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Okta" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Okta identity lookup and user lifecycle management."
}

// CredentialSchema declares the fields required to authenticate against an
// Okta tenant.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credBaseURL, Label: "Base URL", Type: "string", Required: true, Description: "e.g. https://your-org.okta.com"},
		{Key: credAPIToken, Label: "API Token", Type: "string", Required: true, Secret: true},
	}
}

// Actions returns the operator-invokable actions exposed by the Okta plugin.
func (p *Plugin) Actions() []domain.ActionDefinition {
	return []domain.ActionDefinition{
		{Key: actionDeactivate, Label: "Deactivate user", Description: "Permanently deactivate the Okta user account.", Destructive: true, RequiresApproval: true},
		{Key: actionActivate, Label: "Activate user", Description: "Re-activate a previously deactivated Okta user.", Destructive: false},
		{Key: actionSuspend, Label: "Block access (suspend)", Description: "Suspend the user so they cannot authenticate.", Destructive: true, RequiresApproval: true},
	}
}

// HealthCheck calls GET /api/v1/users/me to confirm the token is accepted.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	base, err := requireBaseURL(creds)
	if err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, httpx.JoinURL(base, "/api/v1/users/me"), nil)
	if err != nil {
		return err
	}
	authHeader(req, creds)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// LookupIdentity resolves an email to the upstream Okta user record.
func (p *Plugin) LookupIdentity(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, email string) (domain.Identity, error) {
	base, err := requireBaseURL(creds)
	if err != nil {
		return domain.Identity{}, err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, httpx.JoinURL(base, "/api/v1/users", url.PathEscape(email)), nil)
	if err != nil {
		return domain.Identity{}, err
	}
	authHeader(req, creds)
	var user map[string]any
	if err := httpx.DoJSON(ctx, p.client, req, &user); err != nil {
		return domain.Identity{}, err
	}
	return domain.Identity{
		Email:       email,
		DisplayName: displayNameFromUser(user),
		InstanceID:  instanceID,
		Data:        user,
		FetchedAt:   nowFn(),
	}, nil
}

// Execute runs one of the Okta lifecycle actions against the user identified
// by targetEmail. The Okta API will accept the user's login (email) in place
// of the internal user id.
func (p *Plugin) Execute(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	base, err := requireBaseURL(creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	var op string
	switch actionKey {
	case actionDeactivate:
		op = "deactivate"
	case actionActivate:
		op = "activate"
	case actionSuspend:
		op = "suspend"
	default:
		return domain.ActionResult{}, fmt.Errorf("okta: unknown action %q", actionKey)
	}
	endpoint := httpx.JoinURL(base, "/api/v1/users", url.PathEscape(targetEmail), "lifecycle", op)
	req, err := httpx.NewJSONRequest(http.MethodPost, endpoint, nil)
	if err != nil {
		return domain.ActionResult{}, err
	}
	authHeader(req, creds)
	if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{
		Success: true,
		Message: fmt.Sprintf("okta: %s on %s", op, targetEmail),
		Data:    map[string]any{"action": op, "email": targetEmail},
	}, nil
}

func authHeader(req *http.Request, creds domain.Credentials) {
	req.Header.Set("Authorization", "SSWS "+creds[credAPIToken])
}

func requireBaseURL(creds domain.Credentials) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(creds[credBaseURL]), "/")
	if base == "" {
		return "", errors.New("okta: base_url credential is required")
	}
	return base, nil
}

func displayNameFromUser(user map[string]any) string {
	profile, _ := user["profile"].(map[string]any)
	if profile == nil {
		return ""
	}
	first, _ := profile["firstName"].(string)
	last, _ := profile["lastName"].(string)
	full := strings.TrimSpace(first + " " + last)
	if full != "" {
		return full
	}
	if dn, ok := profile["displayName"].(string); ok {
		return dn
	}
	return ""
}
