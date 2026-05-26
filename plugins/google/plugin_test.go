package google

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/plugin"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Plugin, domain.Credentials, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := New()
	p.client = srv.Client()
	p.adminBaseURL = srv.URL
	p.tokenFn = func(ctx context.Context, sa []byte, admin string, scopes []string) (string, error) {
		return "stub-token", nil
	}
	creds := domain.Credentials{
		credServiceAccount: "{}",
		credAdminEmail:     "admin@example.com",
	}
	return p, creds, srv
}

func TestGoogle_HealthCheck(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/directory/v1/users" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("maxResults") != "1" {
			t.Errorf("maxResults = %q", r.URL.Query().Get("maxResults"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer stub-token" {
			t.Errorf("auth = %q", got)
		}
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestGoogle_LookupIdentity(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := map[string]any{
			"primaryEmail": "ada@example.com",
			"name":         map[string]any{"fullName": "Ada Lovelace"},
		}
		_ = json.NewEncoder(w).Encode(body)
	})
	nowFn = func() time.Time { return time.Unix(2000, 0).UTC() }
	defer func() { nowFn = time.Now }()
	id, err := p.LookupIdentity(context.Background(), creds, uuid.New(), "ada@example.com")
	if err != nil {
		t.Fatalf("LookupIdentity: %v", err)
	}
	if id.DisplayName != "Ada Lovelace" {
		t.Errorf("DisplayName = %q", id.DisplayName)
	}
}

func TestGoogle_PatchActions(t *testing.T) {
	cases := []struct {
		name     string
		action   string
		wantBody map[string]any
	}{
		{"suspend", actionSuspend, map[string]any{"suspended": true}},
		{"activate", actionActivate, map[string]any{"suspended": false}},
		{"archive", actionArchive, map[string]any{"archived": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					t.Errorf("method = %q", r.Method)
				}
				raw, _ := io.ReadAll(r.Body)
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				for k, v := range tc.wantBody {
					if body[k] != v {
						t.Errorf("body[%q] = %v want %v", k, body[k], v)
					}
				}
			})
			res, err := p.Execute(context.Background(), creds, uuid.New(), tc.action, "u@example.com", nil)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
		})
	}
}

func TestGoogle_ResetPassword(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if _, ok := body["password"].(string); !ok {
			t.Errorf("body missing password: %v", body)
		}
		if body["changePasswordAtNextLogin"] != true {
			t.Errorf("changePasswordAtNextLogin not set")
		}
	})
	res, err := p.Execute(context.Background(), creds, uuid.New(), actionResetPassword, "u@example.com", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	pw, _ := res.Data["generated_password"].(string)
	if pw == "" {
		t.Fatal("generated_password missing")
	}
}

func TestGoogle_ClearSessions(t *testing.T) {
	p, creds, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/signOut") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
	})
	res, err := p.Execute(context.Background(), creds, uuid.New(), actionClearSessions, "u@example.com", nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("not success: %+v", res)
	}
}

func TestGoogle_HoldNotConfigured(t *testing.T) {
	p := New()
	creds := domain.Credentials{credServiceAccount: "{}", credAdminEmail: "admin@example.com"}
	_, err := p.PlaceHold(context.Background(), creds, uuid.New(), "u@example.com")
	if !errors.Is(err, plugin.ErrHoldNotConfigured) {
		t.Fatalf("expected ErrHoldNotConfigured, got %v", err)
	}
}

type stubHoldDisp struct {
	placed, removed []uuid.UUID
}

func (s *stubHoldDisp) PlaceHold(ctx context.Context, id uuid.UUID, email string) (domain.ActionResult, error) {
	s.placed = append(s.placed, id)
	return domain.ActionResult{Success: true}, nil
}
func (s *stubHoldDisp) RemoveHold(ctx context.Context, id uuid.UUID, email string) (domain.ActionResult, error) {
	s.removed = append(s.removed, id)
	return domain.ActionResult{Success: true}, nil
}

func TestGoogle_HoldDelegates(t *testing.T) {
	p := New()
	disp := &stubHoldDisp{}
	p.SetHoldDispatcher(disp)
	vaultID := uuid.New()
	creds := domain.Credentials{
		credServiceAccount:   "{}",
		credAdminEmail:       "admin@example.com",
		credVaultInstanceID: vaultID.String(),
	}
	if _, err := p.PlaceHold(context.Background(), creds, uuid.New(), "u@example.com"); err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}
	if len(disp.placed) != 1 || disp.placed[0] != vaultID {
		t.Fatalf("expected delegate to vault %v, got %v", vaultID, disp.placed)
	}
	if _, err := p.RemoveHold(context.Background(), creds, uuid.New(), "u@example.com"); err != nil {
		t.Fatalf("RemoveHold: %v", err)
	}
	if len(disp.removed) != 1 || disp.removed[0] != vaultID {
		t.Fatalf("expected delegate to vault %v, got %v", vaultID, disp.removed)
	}
}
