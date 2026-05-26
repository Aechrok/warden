// Package google_vault implements the Warden plugin for Google Vault. It
// is a HoldProvider only: it manages matters and custodian holds covering
// MAIL, DRIVE, and CHAT for an email custodian.
package google_vault

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
	credServiceAccount = "service_account_json"
	credAdminEmail     = "admin_email"

	vaultBaseURL = "https://vault.googleapis.com"

	scopeVault = "https://www.googleapis.com/auth/ediscovery"
)

// holdCorpora are the Vault corpus types covered by a Warden-managed hold.
var holdCorpora = []string{"MAIL", "DRIVE", "GROUPS"}

// Plugin is the Google Vault HoldProvider. It does not expose actions to
// operators directly.
type Plugin struct {
	client       *http.Client
	vaultBaseURL string
	tokenFn      tokenFunc
}

type tokenFunc func(ctx context.Context, sa []byte, adminEmail string, scopes []string) (string, error)

// New constructs the Plugin with the default HTTP client and live token
// source.
func New() *Plugin {
	return &Plugin{
		client:       httpx.NewClient(),
		vaultBaseURL: vaultBaseURL,
		tokenFn:      liveToken,
	}
}

func init() { plugin.Register(New()) }

// ID returns the stable plugin identifier.
func (p *Plugin) ID() string { return "google_vault" }

// Name returns a human-readable plugin name.
func (p *Plugin) Name() string { return "Google Vault" }

// Description returns a short description of the plugin's capabilities.
func (p *Plugin) Description() string {
	return "Google Vault legal hold orchestration for MAIL, DRIVE, and GROUPS."
}

// CredentialSchema declares the fields required to authenticate against
// Google Vault.
func (p *Plugin) CredentialSchema() []domain.CredentialField {
	return []domain.CredentialField{
		{Key: credServiceAccount, Label: "Service Account JSON", Type: "json", Required: true, Secret: true},
		{Key: credAdminEmail, Label: "Admin Email", Type: "string", Required: true},
	}
}

// HealthCheck probes the matters.list endpoint with a single-page response.
func (p *Plugin) HealthCheck(ctx context.Context, creds domain.Credentials) error {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return err
	}
	req, err := httpx.NewJSONRequest(http.MethodGet, p.vaultBaseURL+"/v1/matters?pageSize=1", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

// PlaceHold ensures a Warden-managed matter exists, then creates one hold
// per corpus for the custodian. The matter name is `warden-{holdID}`; here
// holdID is derived from instanceID, since the plugin layer does not see
// the Hold aggregate ID directly. The cascade layer is expected to call
// PlaceHold per (hold, custodian, instance) tuple.
func (p *Plugin) PlaceHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	matterName := matterNameFor(instanceID)
	matterID, err := p.ensureMatter(ctx, tok, matterName)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	holds, err := p.listHolds(ctx, tok, matterID)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	created := []string{}
	for _, corpus := range holdCorpora {
		holdName := holdNameFor(custodianEmail, corpus)
		if _, ok := holds[holdName]; ok {
			continue
		}
		if err := p.createHold(ctx, tok, matterID, holdName, corpus, custodianEmail); err != nil {
			return domain.ActionResult{Success: false, Message: err.Error()}, err
		}
		created = append(created, corpus)
	}
	return domain.ActionResult{
		Success: true,
		Message: fmt.Sprintf("google_vault: placed hold on %s in matter %s", custodianEmail, matterID),
		Data: map[string]any{
			"matter_id":     matterID,
			"matter_name":   matterName,
			"created":       created,
			"custodian":     custodianEmail,
		},
	}, nil
}

// RemoveHold removes the custodian from every Warden-managed hold in the
// matter. The matter itself is left intact (Vault matters are cheap and
// closing one is irreversible from the API's perspective).
func (p *Plugin) RemoveHold(ctx context.Context, creds domain.Credentials, instanceID uuid.UUID, custodianEmail string) (domain.ActionResult, error) {
	tok, err := p.accessToken(ctx, creds)
	if err != nil {
		return domain.ActionResult{}, err
	}
	matterName := matterNameFor(instanceID)
	matterID, err := p.findMatter(ctx, tok, matterName)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	if matterID == "" {
		// Nothing to remove; idempotent success.
		return domain.ActionResult{Success: true, Message: "google_vault: no matter for instance, nothing to remove"}, nil
	}
	holds, err := p.listHolds(ctx, tok, matterID)
	if err != nil {
		return domain.ActionResult{Success: false, Message: err.Error()}, err
	}
	removed := []string{}
	for _, corpus := range holdCorpora {
		holdName := holdNameFor(custodianEmail, corpus)
		holdID, ok := holds[holdName]
		if !ok {
			continue
		}
		if err := p.deleteHold(ctx, tok, matterID, holdID); err != nil {
			return domain.ActionResult{Success: false, Message: err.Error()}, err
		}
		removed = append(removed, corpus)
	}
	return domain.ActionResult{
		Success: true,
		Message: fmt.Sprintf("google_vault: removed hold for %s from matter %s", custodianEmail, matterID),
		Data: map[string]any{
			"matter_id": matterID,
			"removed":   removed,
			"custodian": custodianEmail,
		},
	}, nil
}

func (p *Plugin) accessToken(ctx context.Context, creds domain.Credentials) (string, error) {
	sa := strings.TrimSpace(creds[credServiceAccount])
	admin := strings.TrimSpace(creds[credAdminEmail])
	if sa == "" || admin == "" {
		return "", errors.New("google_vault: service_account_json and admin_email are required")
	}
	return p.tokenFn(ctx, []byte(sa), admin, []string{scopeVault})
}

func (p *Plugin) ensureMatter(ctx context.Context, token, matterName string) (string, error) {
	if id, err := p.findMatter(ctx, token, matterName); err != nil {
		return "", err
	} else if id != "" {
		return id, nil
	}
	req, err := httpx.NewJSONRequest(http.MethodPost, p.vaultBaseURL+"/v1/matters", map[string]any{
		"name":        matterName,
		"description": "Created by Warden",
	})
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var resp struct {
		MatterID string `json:"matterId"`
		Name     string `json:"name"`
	}
	if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
		return "", err
	}
	if resp.MatterID == "" {
		return "", errors.New("google_vault: matter creation returned no id")
	}
	return resp.MatterID, nil
}

func (p *Plugin) findMatter(ctx context.Context, token, matterName string) (string, error) {
	q := url.Values{"pageSize": []string{"100"}, "state": []string{"OPEN"}}
	endpoint := p.vaultBaseURL + "/v1/matters?" + q.Encode()
	for {
		req, err := httpx.NewJSONRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		var resp struct {
			Matters []struct {
				MatterID string `json:"matterId"`
				Name     string `json:"name"`
			} `json:"matters"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
			return "", err
		}
		for _, m := range resp.Matters {
			if m.Name == matterName {
				return m.MatterID, nil
			}
		}
		if resp.NextPageToken == "" {
			return "", nil
		}
		q.Set("pageToken", resp.NextPageToken)
		endpoint = p.vaultBaseURL + "/v1/matters?" + q.Encode()
	}
}

func (p *Plugin) listHolds(ctx context.Context, token, matterID string) (map[string]string, error) {
	out := map[string]string{}
	endpoint := p.vaultBaseURL + "/v1/matters/" + url.PathEscape(matterID) + "/holds?pageSize=100"
	for {
		req, err := httpx.NewJSONRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		var resp struct {
			Holds []struct {
				HoldID string `json:"holdId"`
				Name   string `json:"name"`
			} `json:"holds"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := httpx.DoJSON(ctx, p.client, req, &resp); err != nil {
			return nil, err
		}
		for _, h := range resp.Holds {
			out[h.Name] = h.HoldID
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		endpoint = p.vaultBaseURL + "/v1/matters/" + url.PathEscape(matterID) + "/holds?pageSize=100&pageToken=" + url.QueryEscape(resp.NextPageToken)
	}
}

func (p *Plugin) createHold(ctx context.Context, token, matterID, holdName, corpus, custodian string) error {
	body := map[string]any{
		"name":   holdName,
		"corpus": corpus,
		"accounts": []map[string]any{
			{"email": custodian},
		},
	}
	endpoint := p.vaultBaseURL + "/v1/matters/" + url.PathEscape(matterID) + "/holds"
	req, err := httpx.NewJSONRequest(http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

func (p *Plugin) deleteHold(ctx context.Context, token, matterID, holdID string) error {
	endpoint := p.vaultBaseURL + "/v1/matters/" + url.PathEscape(matterID) + "/holds/" + url.PathEscape(holdID)
	req, err := httpx.NewJSONRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return httpx.DoJSON(ctx, p.client, req, nil)
}

func matterNameFor(instanceID uuid.UUID) string {
	return "warden-" + instanceID.String()
}

func holdNameFor(email, corpus string) string {
	// Vault hold names must match [a-zA-Z0-9_-]; replace email punctuation.
	safe := strings.NewReplacer("@", "_at_", ".", "_").Replace(email)
	return "warden-" + safe + "-" + strings.ToLower(corpus)
}
