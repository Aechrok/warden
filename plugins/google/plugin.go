// Package google implements the Warden plugin for Google Workspace. It
// covers user lifecycle (suspend, activate, archive, reset_password,
// clear_sessions), identity lookup, and delegates legal hold operations to
// a linked google_vault instance when configured.
package google

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	credServiceAccount   = "service_account_json"
	credAdminEmail       = "admin_email"
	credDomain           = "domain"
	credVaultInstanceID  = "vault_instance_id"

	actionSuspend       = "suspend_user"
	actionActivate      = "activate_user"
	actionArchive       = "archive_user"
	actionResetPassword = "reset_password"
	actionClearSessions = "clear_sessions"

	adminBaseURL = "https://admin.googleapis.com"

	scopeUserDirectory = "https://www.googleapis.com/auth/admin.directory.user"
)

// HoldDispatcher is the interface used by Plugin to forward hold operations
// to a linked google_vault instance. Wire the package's Dispatcher with
// SetHoldDispatcher before any hold call is made; if unset, hold calls
// return ErrHoldNotConfigured.
type HoldDispatcher interface {
	PlaceHold(ctx context.Context, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error)
	RemoveHold(ctx context.Context, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error)
}

// Plugin implements domain.Plugin, ActionExecutor, IdentityProvider, and
// HoldProvider for Google Workspace.
type Plugin struct {
	client       *http.Client
	adminBaseURL string
	tokenFn      tokenFunc
	holdDisp     HoldDispatcher
}

type tokenFunc func(ctx context.Context, sa []byte, adminEmail string, scopes []string) (string, error)

// New constructs the Plugin with the default HTTP client and live
// service-account token source.
func New() *Plugin {
	return &Plugin{
		client:       httpx.NewClient(),
		adminBaseURL: adminBaseURL,
		tokenFn:      liveToken,
	}
}

func init() { plugin.Register(New()) }

// SetHoldDispatcher wires the plugin to a dispatcher capable of resolving a
// linked google_vault instance. The control plane should call this once at
// startup after the global Dispatcher is constructed.
func (p *Plugin) SetHoldDispatcher(d HoldDispatcher) { p.holdDisp = d }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "google_workspace" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Google Workspace" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Google Workspace user lifecycle and identity lookup via the Admin SDK."
}

// CredentialSchema declares the fields required to authenticate against a
// Google Workspace tenant.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credServiceAccount, Label: "Service Account JSON", Type: "json", Required: true, Secret: true, Description: "Full JSON key for a domain-wide-delegated service account."},
		{Key: credAdminEmail, Label: "Admin Email", Type: "string", Required: true, Description: "Workspace admin to impersonate via DWD."},
		{Key: credDomain, Label: "Primary Domain", Type: "string", Required: false, Description: "Used by HealthCheck to scope the users.list probe."},
		{Key: credVaultInstanceID, Label: "Linked Vault Instance ID", Type: "string", Required: false, Description: "UUID of a google_vault instance to delegate hold operations to."},
	}
}

// Actions returns the operator-invokable actions for the Workspace plugin.
func (p *Plugin) Actions() []domain.ActionDefinition {
	return []domain.ActionDefinition{
		{Key: actionSuspend, Label: "Suspend user", Description: "Suspend the Workspace account; signs out all sessions.", Destructive: true, RequiresApproval: true},
		{Key: actionActivate, Label: "Activate user", Description: "Lift suspension on the Workspace account.", Destructive: false},
		{Key: actionArchive, Label: "Archive user", Description: "Move the user to an archived state (retains data but disables sign-in).", Destructive: true, RequiresApproval: true},
		{Key: actionResetPassword, Label: "Reset password", Description: "Generate a new password and force change at next sign-in.", Destructive: false},
		{Key: actionClearSessions, Label: "Clear sessions", Description: "Sign the user out of all active sessions.", Destructive: false},
	}
}

// HealthCheck probes the Admin SDK users.list endpoint with a single-row
// page to confirm credentials and DWD setup.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return err
	}
	q := url.Values{"maxResults": []string{"1"}}
	if d := strings.TrimSpace(creds[credDomain]); d != "" {
		q.Set("domain", d)
	} else {
		q.Set("customer", "my_customer")
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.adminBaseURL+"/admin/directory/v1/users?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// LookupIdentity returns the Workspace directory record for email.
func (p *Plugin) LookupIdentity(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, email string) (domain.Identity, error) {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.Identity{}, err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.adminBaseURL+"/admin/directory/v1/users/"+url.PathEscape(email), nil)
	if err != nil {
		return domain.Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
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

// Execute dispatches one of the user-lifecycle actions.
func (p *Plugin) Execute(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}

	switch actionKey {
	case actionSuspend:
		return p.patchUser(ctx, tok, targetEmail, map[string]any{"suspended": true}, "suspended")
	case actionActivate:
		return p.patchUser(ctx, tok, targetEmail, map[string]any{"suspended": false}, "activated")
	case actionArchive:
		return p.patchUser(ctx, tok, targetEmail, map[string]any{"archived": true}, "archived")
	case actionResetPassword:
		pw, err := generatePassword(20)
		if err != nil {
			return domain.ActionResult{}, fmt.Errorf("google: generate password: %w", err)
		}
		res, err := p.patchUser(ctx, tok, targetEmail, map[string]any{
			"password":                  pw,
			"changePasswordAtNextLogin": true,
		}, "password_reset")
		if err != nil {
			return res, err
		}
		if res.Data == nil {
			res.Data = map[string]any{}
		}
		res.Data["generated_password"] = pw
		return res, nil
	case actionClearSessions:
		req, err := httpx.NewJSONRequest(http.MethodPost, p.adminBaseURL+"/admin/directory/v1/users/"+url.PathEscape(targetEmail)+"/signOut", nil)
		if err != nil {
			return domain.ActionResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
			return domain.ActionResult{Success: false, Message: err.Error()}, err
		}
		return domain.ActionResult{Success: true, Message: "google: sessions cleared for " + targetEmail, Data: map[string]any{"action": "clear_sessions", "email": targetEmail}}, nil
	default:
		return domain.ActionResult{}, fmt.Errorf("google: unknown action %q", actionKey)
	}
}

// PlaceHold delegates to the linked google_vault instance, if configured.
func (p *Plugin) PlaceHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	vaultID, err := vaultInstanceID(creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	if p.holdDisp == nil {
		return domain.ActionResult{}, errors.New("google: hold dispatcher not wired")
	}
	return p.holdDisp.PlaceHold(ctx, vaultID, custodianEmail)
}

// RemoveHold delegates to the linked google_vault instance, if configured.
func (p *Plugin) RemoveHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	vaultID, err := vaultInstanceID(creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	if p.holdDisp == nil {
		return domain.ActionResult{}, errors.New("google: hold dispatcher not wired")
	}
	return p.holdDisp.RemoveHold(ctx, vaultID, custodianEmail)
}

func (p *Plugin) patchUser(ctx context.Context, token, email string, body map[string]any, label string) (domain.ActionResult, error) {
	endpoint := p.adminBaseURL + "/admin/directory/v1/users/" + url.PathEscape(email)
	req, err := httpx.NewJSONRequest(http.MethodPatch, endpoint, body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{
		Success: true,
		Message: "google: " + label + " " + email,
		Data:    map[string]any{"action": label, "email": email},
	}, nil
}

func (p *Plugin) accessToken(ctx context.Context, creds domain.Credentials) (string, error) {
	sa := strings.TrimSpace(creds[credServiceAccount])
	admin := strings.TrimSpace(creds[credAdminEmail])
	if sa == "" || admin == "" {
		return "", errors.New("google: service_account_json and admin_email are required")
	}
	return p.tokenFn(ctx, []byte(sa), admin, []string{scopeUserDirectory})
}

func vaultInstanceID(creds domain.Credentials) (uuid.UUID, error) {
	raw := strings.TrimSpace(creds[credVaultInstanceID])
	if raw == "" {
		return uuid.Nil, plugin.ErrHoldNotConfigured
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("google: vault_instance_id is not a valid UUID: %w", err)
	}
	return id, nil
}

func displayNameFromUser(user map[string]any) string {
	if name, ok := user["name"].(map[string]any); ok {
		if full, ok := name["fullName"].(string); ok && full != "" {
			return full
		}
		given, _ := name["givenName"].(string)
		family, _ := name["familyName"].(string)
		full := strings.TrimSpace(given + " " + family)
		if full != "" {
			return full
		}
	}
	if dn, ok := user["displayName"].(string); ok {
		return dn
	}
	return ""
}

// generatePassword returns a URL-safe random password of approximately n
// bytes after base64 encoding. The result is suitable for forcing the user
// to change at next login.
func generatePassword(n int) (string, error) {
	if n < 12 {
		n = 12
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
