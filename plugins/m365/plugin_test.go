package m365

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
)

func newTestPlugin(t *testing.T, handler http.HandlerFunc) (*Plugin, domain.Credentials) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := New()
	p.SetHTTPClient(srv.Client())
	p.SetGraphBaseURL(srv.URL)
	p.SetTokenFunc(func(ctx context.Context, tenant, clientID, secret string) (string, error) {
		return "stub-token", nil
	})
	return p, domain.Credentials{
		credTenantID:    "tenant",
		credClientID:    "client",
		credClientSecret: "secret",
	}
}

func TestM365_HealthCheck(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.0/organization" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("$top") != "1" {
			t.Errorf("$top = %q", r.URL.Query().Get("$top"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer stub-token" {
			t.Errorf("auth = %q", got)
		}
		_, _ = w.Write([]byte("{}"))
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestM365_PlaceAndRemoveHold(t *testing.T) {
	tests := []struct {
		name string
		on   bool
		fn   func(ctx context.Context, p *Plugin, creds domain.Credentials, instanceID uuid.UUID) (domain.ActionResult, error)
	}{
		{"place", true, func(ctx context.Context, p *Plugin, creds domain.Credentials, instanceID uuid.UUID) (domain.ActionResult, error) {
			return p.PlaceHold(ctx, creds, instanceID, "ada@example.com")
		}},
		{"remove", false, func(ctx context.Context, p *Plugin, creds domain.Credentials, instanceID uuid.UUID) (domain.ActionResult, error) {
			return p.RemoveHold(ctx, creds, instanceID, "ada@example.com")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Errorf("method = %q", r.Method)
				}
				if !strings.HasSuffix(r.URL.Path, "/mailboxSettings") {
					t.Errorf("path = %q", r.URL.Path)
				}
				raw, _ := io.ReadAll(r.Body)
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				if body["litigationHoldEnabled"] != tc.on {
					t.Errorf("litigationHoldEnabled = %v, want %v", body["litigationHoldEnabled"], tc.on)
				}
			})
			res, err := tc.fn(context.Background(), p, creds, uuid.New())
			if err != nil {
				t.Fatalf("hold: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
		})
	}
}

func TestM365_ComplianceHoldEnabled(t *testing.T) {
	calls := []string{}
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
	})
	creds[credCompliance] = "true"
	if _, err := p.PlaceHold(context.Background(), creds, uuid.New(), "ada@example.com"); err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (mailbox + compliance), got %d: %v", len(calls), calls)
	}
}

func TestM365_MissingCreds(t *testing.T) {
	p := New()
	if err := p.HealthCheck(context.Background(), domain.Credentials{}); err == nil {
		t.Fatal("expected error on missing creds")
	}
}
