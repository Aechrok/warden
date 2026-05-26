package okta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Plugin, domain.Credentials, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := New()
	p.client = srv.Client()
	creds := domain.Credentials{credBaseURL: srv.URL, credAPIToken: "test-token"}
	return p, creds, srv
}

func TestOkta_HealthCheck(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/me" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "SSWS test-token" {
			t.Errorf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{"id":"me"}`))
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestOkta_LookupIdentity(t *testing.T) {
	body := map[string]any{
		"id":      "00uabc",
		"profile": map[string]any{"firstName": "Ada", "lastName": "Lovelace"},
	}
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/users/") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	nowFn = func() time.Time { return time.Unix(1000, 0).UTC() }
	defer func() { nowFn = time.Now }()
	id, err := p.LookupIdentity(context.Background(), creds, uuid.New(), "ada@example.com")
	if err != nil {
		t.Fatalf("LookupIdentity: %v", err)
	}
	if id.DisplayName != "Ada Lovelace" {
		t.Errorf("DisplayName = %q", id.DisplayName)
	}
	if id.Email != "ada@example.com" {
		t.Errorf("Email = %q", id.Email)
	}
	if id.Data["id"] != "00uabc" {
		t.Errorf("Data not populated: %+v", id.Data)
	}
}

func TestOkta_Execute_Actions(t *testing.T) {
	tests := []struct {
		name      string
		actionKey string
		op        string
	}{
		{"deactivate", actionDeactivate, "deactivate"},
		{"activate", actionActivate, "activate"},
		{"suspend", actionSuspend, "suspend"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantPath := "/api/v1/users/u%40example.com/lifecycle/" + tc.op
			p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %q", r.Method)
				}
				if r.URL.EscapedPath() != wantPath {
					t.Errorf("path = %q want %q", r.URL.EscapedPath(), wantPath)
				}
			})
			res, err := p.Execute(context.Background(), creds, uuid.New(), tc.actionKey, "u@example.com", nil)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
			if res.Data["action"] != tc.op {
				t.Errorf("data.action = %v", res.Data["action"])
			}
		})
	}
}

func TestOkta_Execute_UnknownAction(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := p.Execute(context.Background(), creds, uuid.New(), "nope", "x@example.com", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestOkta_MissingBaseURL(t *testing.T) {
	p := New()
	err := p.HealthCheck(context.Background(), domain.Credentials{credAPIToken: "x"})
	if err == nil {
		t.Fatal("expected error when base_url missing")
	}
}
