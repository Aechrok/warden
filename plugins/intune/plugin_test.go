package intune

import (
	"context"
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

func TestIntune_Actions(t *testing.T) {
	cases := []struct {
		name      string
		actionKey string
		op        string
	}{
		{"wipe", actionWipe, "wipe"},
		{"lock", actionLock, "remoteLock"},
		{"retire", actionRetire, "retire"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantPath := "/v1.0/deviceManagement/managedDevices/dev-1/" + tc.op
			p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %q", r.Method)
				}
				if r.URL.Path != wantPath {
					t.Errorf("path = %q want %q", r.URL.Path, wantPath)
				}
			})
			res, err := p.Execute(context.Background(), creds, uuid.New(), tc.actionKey, "u@example.com", map[string]any{paramDeviceID: "dev-1"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
		})
	}
}

func TestIntune_MissingDeviceID(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := p.Execute(context.Background(), creds, uuid.New(), actionWipe, "u@example.com", nil)
	if err == nil || !strings.Contains(err.Error(), "device_id") {
		t.Fatalf("expected device_id error, got %v", err)
	}
}

func TestIntune_HealthCheck(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "%24top=1") && r.URL.Query().Get("$top") != "1" {
			t.Errorf("missing $top=1: %q", r.URL.RawQuery)
		}
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}
