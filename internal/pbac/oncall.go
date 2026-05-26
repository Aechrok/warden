package pbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aechrok/warden/internal/config"
)

// OnCallResolver answers the question "is this operator currently on call?"
// for the on_call_verification policy. Implementations must be safe for
// concurrent use; the resolver is typically a singleton created at startup.
type OnCallResolver interface {
	IsOnCall(ctx context.Context, email string) (bool, error)
}

// NewResolver constructs the resolver dictated by the ON_CALL_PROVIDER
// configuration value. Recognized values:
//
//   - "none" (default) → NoneResolver, always returns true
//   - "pagerduty"      → PagerDutyResolver
//   - "opsgenie"       → OpsGenieResolver
//
// Any other value falls back to NoneResolver so a typo in env config does
// not silently block every destructive action.
func NewResolver(cfg *config.Config) OnCallResolver {
	if cfg == nil {
		return NoneResolver{}
	}
	switch strings.ToLower(strings.TrimSpace(cfg.OnCallProvider)) {
	case "pagerduty":
		return &PagerDutyResolver{APIKey: cfg.OnCallAPIKey, HTTP: defaultHTTPClient()}
	case "opsgenie":
		return &OpsGenieResolver{APIKey: cfg.OnCallAPIKey, HTTP: defaultHTTPClient()}
	default:
		return NoneResolver{}
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}

// NoneResolver returns true for every caller. Used when on-call data is not
// available; the on_call_verification policy effectively becomes a no-op
// because every operator is reported as "on call".
type NoneResolver struct{}

// IsOnCall implements OnCallResolver.
func (NoneResolver) IsOnCall(_ context.Context, _ string) (bool, error) { return true, nil }

// PagerDutyResolver queries the PagerDuty /oncalls endpoint. A user is
// considered on-call if any oncall record exists whose user email matches.
// Reference: https://developer.pagerduty.com/api-reference/.
type PagerDutyResolver struct {
	APIKey  string
	HTTP    *http.Client
	BaseURL string // defaults to https://api.pagerduty.com
}

// IsOnCall implements OnCallResolver.
func (r *PagerDutyResolver) IsOnCall(ctx context.Context, email string) (bool, error) {
	if r == nil {
		return false, errors.New("pbac: nil pagerduty resolver")
	}
	if r.APIKey == "" {
		return false, errors.New("pbac: pagerduty: missing api key")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}

	base := r.BaseURL
	if base == "" {
		base = "https://api.pagerduty.com"
	}

	// Resolve user by email first.
	userID, err := r.lookupUserID(ctx, base, email)
	if err != nil {
		return false, err
	}
	if userID == "" {
		return false, nil
	}

	endpoint := fmt.Sprintf("%s/oncalls?user_ids[]=%s&limit=1", base, url.QueryEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("pbac: pagerduty: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+r.APIKey)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

	resp, err := r.client().Do(req)
	if err != nil {
		return false, fmt.Errorf("pbac: pagerduty: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pbac: pagerduty: status %d", resp.StatusCode)
	}

	var body struct {
		Oncalls []struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"oncalls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("pbac: pagerduty: decode: %w", err)
	}
	return len(body.Oncalls) > 0, nil
}

func (r *PagerDutyResolver) lookupUserID(ctx context.Context, base, email string) (string, error) {
	endpoint := fmt.Sprintf("%s/users?query=%s&limit=1", base, url.QueryEscape(email))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("pbac: pagerduty users: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token token="+r.APIKey)
	req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

	resp, err := r.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("pbac: pagerduty users: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pbac: pagerduty users: status %d", resp.StatusCode)
	}

	var body struct {
		Users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("pbac: pagerduty users: decode: %w", err)
	}
	for _, u := range body.Users {
		if strings.EqualFold(u.Email, email) {
			return u.ID, nil
		}
	}
	return "", nil
}

func (r *PagerDutyResolver) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return defaultHTTPClient()
}

// OpsGenieResolver queries OpsGenie's /v2/schedules/on-calls endpoint and
// checks whether the supplied email appears in any participant list.
// Reference: https://docs.opsgenie.com/docs/who-is-on-call-api.
type OpsGenieResolver struct {
	APIKey  string
	HTTP    *http.Client
	BaseURL string // defaults to https://api.opsgenie.com
}

// IsOnCall implements OnCallResolver.
func (r *OpsGenieResolver) IsOnCall(ctx context.Context, email string) (bool, error) {
	if r == nil {
		return false, errors.New("pbac: nil opsgenie resolver")
	}
	if r.APIKey == "" {
		return false, errors.New("pbac: opsgenie: missing api key")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}

	base := r.BaseURL
	if base == "" {
		base = "https://api.opsgenie.com"
	}

	endpoint := fmt.Sprintf("%s/v2/schedules/on-calls?flat=true", base)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("pbac: opsgenie: build request: %w", err)
	}
	req.Header.Set("Authorization", "GenieKey "+r.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := r.client().Do(req)
	if err != nil {
		return false, fmt.Errorf("pbac: opsgenie: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("pbac: opsgenie: status %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			OnCallRecipients []string `json:"onCallRecipients"`
			OnCallParticipants []struct {
				Name string `json:"name"`
			} `json:"onCallParticipants"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("pbac: opsgenie: decode: %w", err)
	}
	for _, s := range body.Data {
		for _, recipient := range s.OnCallRecipients {
			if strings.EqualFold(recipient, email) {
				return true, nil
			}
		}
		for _, p := range s.OnCallParticipants {
			if strings.EqualFold(p.Name, email) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *OpsGenieResolver) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return defaultHTTPClient()
}
