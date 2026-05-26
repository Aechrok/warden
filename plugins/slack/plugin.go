// Package slack implements the Warden plugin for Slack. It supports user
// lifecycle via SCIM, session invalidation via admin.users.session.invalidate,
// identity lookup via users.lookupByEmail, and legal holds via the Enterprise
// Grid admin.legalHolds endpoints.
package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/plugins/internal/httpx"
)

const (
	credBotToken  = "bot_token"
	credUserToken = "user_token"

	actionDeactivate = "deactivate_user"
	actionReactivate = "reactivate_user"
	actionClearSess  = "clear_sessions"

	defaultBaseURL = "https://slack.com"
)

// Plugin implements Plugin, ActionExecutor, IdentityProvider, and
// HoldProvider for Slack.
type Plugin struct {
	client  *http.Client
	baseURL string
}

// New constructs the Plugin with the default HTTP client.
func New() *Plugin {
	return &Plugin{client: httpx.NewClient(), baseURL: defaultBaseURL}
}

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "slack" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Slack" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Slack identity lookup, user lifecycle, session invalidation, and legal holds."
}

// CredentialSchema declares the fields required to authenticate against
// Slack.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credBotToken, Label: "Bot Token", Type: "string", Required: true, Secret: true, Description: "xoxb-...; used for users.lookupByEmail and auth.test."},
		{Key: credUserToken, Label: "User Token", Type: "string", Required: true, Secret: true, Description: "xoxp-... or admin user token; used for SCIM and admin.* methods."},
	}
}

// Actions returns the operator-invokable actions exposed by the Slack plugin.
func (p *Plugin) Actions() []domain.ActionDefinition {
	return []domain.ActionDefinition{
		{Key: actionDeactivate, Label: "Deactivate user", Description: "Deactivate the Slack user via SCIM.", Destructive: true, RequiresApproval: true},
		{Key: actionReactivate, Label: "Reactivate user", Description: "Re-activate a previously deactivated Slack user.", Destructive: false},
		{Key: actionClearSess, Label: "Clear sessions", Description: "Invalidate all of the user's active Slack sessions.", Destructive: false},
	}
}

// HealthCheck calls api/auth.test with the bot token.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	bot := creds[credBotToken]
	if bot == "" {
		return errors.New("slack: bot_token is required")
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.baseURL+"/api/auth.test", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bot)
	var resp slackResp
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return err
	}
	return resp.checkOK("auth.test")
}

// LookupIdentity calls users.lookupByEmail to resolve email -> user.
func (p *Plugin) LookupIdentity(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, email string) (domain.Identity, error) {
	bot := creds[credBotToken]
	if bot == "" {
		return domain.Identity{}, errors.New("slack: bot_token is required")
	}
	q := url.Values{"email": []string{email}}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.baseURL+"/api/users.lookupByEmail?"+q.Encode(), nil)
	if err != nil {
		return domain.Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+bot)
	var resp struct {
		slackResp
		User map[string]any `json:"user"`
	}
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return domain.Identity{}, err
	}
	if err := resp.checkOK("users.lookupByEmail"); err != nil {
		return domain.Identity{}, err
	}
	return domain.Identity{
		Email:       email,
		DisplayName: displayNameFromUser(resp.User),
		InstanceID:  instanceID,
		Data:        resp.User,
		FetchedAt:   nowFn(),
	}, nil
}

// Execute dispatches one of the user-lifecycle actions.
func (p *Plugin) Execute(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any) (domain.ActionResult, error) {
	switch actionKey {
	case actionDeactivate:
		return p.scimSetActive(ctx, creds, targetEmail, false)
	case actionReactivate:
		return p.scimSetActive(ctx, creds, targetEmail, true)
	case actionClearSess:
		return p.clearSessions(ctx, creds, targetEmail)
	default:
		return domain.ActionResult{}, fmt.Errorf("slack: unknown action %q", actionKey)
	}
}

// PlaceHold creates a legal hold for the custodian. Hold name format:
// `warden-{instanceID}-{email}`.
func (p *Plugin) PlaceHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	user := creds[credUserToken]
	if user == "" {
		return domain.ActionResult{}, errors.New("slack: user_token is required")
	}
	userID, err := p.userIDForEmail(ctx, creds, custodianEmail)
	if err != nil {
		return domain.ActionResult{}, err
	}
	holdName := holdNameFor(instanceID, custodianEmail)
	body := map[string]any{
		"name":          holdName,
		"user_ids":      []string{userID},
		"start_date_ts": time.Now().Unix(),
	}
	req, err := httpx.NewJSONRequest(http.MethodPost, p.baseURL+"/api/admin.legalHolds.create", body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+user)
	var resp struct {
		slackResp
		LegalHold map[string]any `json:"legal_hold"`
	}
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	if err := resp.checkOK("admin.legalHolds.create"); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{
		Success: true,
		Message: "slack: placed hold " + holdName,
		Data:    map[string]any{"hold_name": holdName, "user_id": userID, "legal_hold": resp.LegalHold},
	}, nil
}

// RemoveHold finds the matching hold by name and releases it.
func (p *Plugin) RemoveHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	user := creds[credUserToken]
	if user == "" {
		return domain.ActionResult{}, errors.New("slack: user_token is required")
	}
	holdName := holdNameFor(instanceID, custodianEmail)
	id, err := p.findHoldID(ctx, user, holdName)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	if id == "" {
		return domain.ActionResult{Success: true, Message: "slack: no matching hold to release"}, nil
	}
	body := map[string]any{"legal_hold_id": id}
	req, err := httpx.NewJSONRequest(http.MethodPost, p.baseURL+"/api/admin.legalHolds.releaseHold", body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+user)
	var resp slackResp
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	if err := resp.checkOK("admin.legalHolds.releaseHold"); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{Success: true, Message: "slack: released hold " + holdName, Data: map[string]any{"hold_id": id}}, nil
}

func (p *Plugin) scimSetActive(ctx context.Context, creds domain.Credentials, email string, active bool) (domain.ActionResult, error) {
	user := creds[credUserToken]
	if user == "" {
		return domain.ActionResult{}, errors.New("slack: user_token is required")
	}
	scimID, err := p.scimIDForEmail(ctx, user, email)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	endpoint := p.baseURL + "/scim/v2/Users/" + url.PathEscape(scimID)
	body := map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{"op": "replace", "path": "active", "value": active},
		},
	}
	req, err := httpx.NewJSONRequest(http.MethodPatch, endpoint, body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+user)
	if err := httpx.DoJSON(ctx, p.client, req, nil); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	label := "deactivated"
	if active {
		label = "reactivated"
	}
	return domain.ActionResult{
		Success: true,
		Message: "slack: " + label + " " + email,
		Data:    map[string]any{"action": label, "email": email, "scim_id": scimID},
	}, nil
}

func (p *Plugin) clearSessions(ctx context.Context, creds domain.Credentials, email string) (domain.ActionResult, error) {
	user := creds[credUserToken]
	if user == "" {
		return domain.ActionResult{}, errors.New("slack: user_token is required")
	}
	userID, err := p.userIDForEmail(ctx, creds, email)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	body := map[string]any{"user_id": userID}
	req, err := httpx.NewJSONRequest(http.MethodPost, p.baseURL+"/api/admin.users.session.invalidate", body)
	if err != nil {
		return domain.ActionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+user)
	var resp slackResp
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	if err := resp.checkOK("admin.users.session.invalidate"); err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	return domain.ActionResult{Success: true, Message: "slack: sessions cleared for " + email, Data: map[string]any{"action": "clear_sessions", "email": email, "user_id": userID}}, nil
}

func (p *Plugin) userIDForEmail(ctx context.Context, creds domain.Credentials, email string) (string, error) {
	id, err := p.LookupIdentity(ctx, creds, uuid.Nil, email)
	if err != nil {
		return "", err
	}
	uid, _ := id.Data["id"].(string)
	if uid == "" {
		return "", fmt.Errorf("slack: lookup returned no id for %s", email)
	}
	return uid, nil
}

func (p *Plugin) scimIDForEmail(ctx context.Context, userToken, email string) (string, error) {
	q := url.Values{"filter": []string{"userName eq \"" + email + "\""}}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.baseURL+"/scim/v2/Users?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	var resp struct {
		Resources []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
		} `json:"Resources"`
	}
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	for _, r := range resp.Resources {
		if strings.EqualFold(r.UserName, email) {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("slack: SCIM lookup returned no user for %s", email)
}

func (p *Plugin) findHoldID(ctx context.Context, userToken, holdName string) (string, error) {
	req, err := httpx.NewJSONRequest(http.MethodGet, p.baseURL+"/api/admin.legalHolds.list", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	var resp struct {
		slackResp
		LegalHolds []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"legal_holds"`
	}
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	if err := resp.checkOK("admin.legalHolds.list"); err != nil {
		return "", err
	}
	for _, h := range resp.LegalHolds {
		if h.Name == holdName {
			return h.ID, nil
		}
	}
	return "", nil
}

type slackResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s slackResp) checkOK(method string) error {
	if s.OK {
		return nil
	}
	if s.Error != "" {
		return fmt.Errorf("slack: %s failed: %s", method, s.Error)
	}
	return fmt.Errorf("slack: %s failed", method)
}

func displayNameFromUser(user map[string]any) string {
	if profile, ok := user["profile"].(map[string]any); ok {
		if real, ok := profile["real_name"].(string); ok && real != "" {
			return real
		}
		if dn, ok := profile["display_name"].(string); ok && dn != "" {
			return dn
		}
	}
	if real, ok := user["real_name"].(string); ok && real != "" {
		return real
	}
	if dn, ok := user["name"].(string); ok {
		return dn
	}
	return ""
}

func holdNameFor(instanceID uuid.UUID, email string) string {
	safe := strings.NewReplacer("@", "_at_", ".", "_").Replace(email)
	return "warden-" + instanceID.String() + "-" + safe
}
