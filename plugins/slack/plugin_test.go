package slack

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
	p.client = srv.Client()
	p.baseURL = srv.URL
	creds := domain.Credentials{credBotToken: "xoxb-bot", credUserToken: "xoxp-user"}
	return p, creds
}

func TestSlack_HealthCheck(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth.test" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestSlack_HealthCheckFails(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_auth"})
	})
	if err := p.HealthCheck(context.Background(), creds); err == nil {
		t.Fatal("expected error on ok:false")
	}
}

func TestSlack_LookupIdentity(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users.lookupByEmail" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"user": map[string]any{
				"id":   "U123",
				"name": "ada",
				"profile": map[string]any{
					"real_name":    "Ada Lovelace",
					"display_name": "ada",
				},
			},
		})
	})
	id, err := p.LookupIdentity(context.Background(), creds, uuid.New(), "ada@example.com")
	if err != nil {
		t.Fatalf("LookupIdentity: %v", err)
	}
	if id.DisplayName != "Ada Lovelace" {
		t.Errorf("DisplayName = %q", id.DisplayName)
	}
	if id.Data["id"] != "U123" {
		t.Errorf("expected U123, got %v", id.Data["id"])
	}
}

func TestSlack_DeactivateAndReactivate(t *testing.T) {
	cases := []struct {
		name   string
		action string
		want   bool
	}{
		{"deactivate", actionDeactivate, false},
		{"reactivate", actionReactivate, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/scim/v2/Users":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"Resources": []map[string]any{{"id": "S-1", "userName": "ada@example.com"}},
					})
				case strings.HasPrefix(r.URL.Path, "/scim/v2/Users/"):
					if r.Method != http.MethodPatch {
						t.Errorf("method = %q", r.Method)
					}
					raw, _ := io.ReadAll(r.Body)
					var body map[string]any
					_ = json.Unmarshal(raw, &body)
					ops, _ := body["Operations"].([]any)
					if len(ops) != 1 {
						t.Fatalf("expected 1 op, got %d", len(ops))
					}
					op, _ := ops[0].(map[string]any)
					if op["value"] != tc.want {
						t.Errorf("active value = %v, want %v", op["value"], tc.want)
					}
				default:
					t.Errorf("unexpected path %q", r.URL.Path)
				}
			})
			res, err := p.Execute(context.Background(), creds, uuid.New(), tc.action, "ada@example.com", nil)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
		})
	}
}

func TestSlack_ClearSessions(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/users.lookupByEmail":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"user": map[string]any{"id": "U9"},
			})
		case "/api/admin.users.session.invalidate":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["user_id"] != "U9" {
				t.Errorf("user_id = %v", body["user_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	res, err := p.Execute(context.Background(), creds, uuid.New(), actionClearSess, "ada@example.com", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("not success: %+v", res)
	}
}

func TestSlack_PlaceAndReleaseHold(t *testing.T) {
	instanceID := uuid.New()
	holdName := holdNameFor(instanceID, "ada@example.com")
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/users.lookupByEmail":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": map[string]any{"id": "U9"}})
		case "/api/admin.legalHolds.create":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["name"] != holdName {
				t.Errorf("name = %v, want %s", body["name"], holdName)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "legal_hold": map[string]any{"id": "H1", "name": holdName}})
		case "/api/admin.legalHolds.list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"legal_holds": []map[string]any{{"id": "H1", "name": holdName}},
			})
		case "/api/admin.legalHolds.releaseHold":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			if body["legal_hold_id"] != "H1" {
				t.Errorf("legal_hold_id = %v", body["legal_hold_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})
	ctx := context.Background()
	if _, err := p.PlaceHold(ctx, creds, instanceID, "ada@example.com"); err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}
	if _, err := p.RemoveHold(ctx, creds, instanceID, "ada@example.com"); err != nil {
		t.Fatalf("RemoveHold: %v", err)
	}
}
