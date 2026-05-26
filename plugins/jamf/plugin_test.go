package jamf

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
	p.SetTokenFunc(func(ctx context.Context, baseURL, clientID, secret string) (string, error) {
		return "stub-token", nil
	})
	return p, domain.Credentials{
		credBaseURL:      srv.URL,
		credClientID:     "client",
		credClientSecret: "secret",
	}
}

func TestJamf_HealthCheck(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer stub-token" {
			t.Errorf("auth = %q", got)
		}
	})
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestJamf_Actions(t *testing.T) {
	cases := []struct {
		name      string
		actionKey string
		path      string
		params    map[string]any
	}{
		{"lock computer", actionLockComputer, "/JSSResource/computercommands/command/DeviceLock/id/5", map[string]any{paramDeviceID: "5"}},
		{"lock computer w/ msg", actionLockComputer, "/JSSResource/computercommands/command/DeviceLock/id/5/locked", map[string]any{paramDeviceID: "5", paramLockMessage: "locked"}},
		{"wipe computer", actionWipeComputer, "/JSSResource/computercommands/command/EraseDevice/id/5", map[string]any{paramDeviceID: "5"}},
		{"wipe computer w/ passcode", actionWipeComputer, "/JSSResource/computercommands/command/EraseDevice/id/5/passcode/123456", map[string]any{paramDeviceID: "5", paramPasscode: "123456"}},
		{"lock mobile", actionLockMobile, "/JSSResource/mobiledevicecommands/command/DeviceLock/id/7", map[string]any{paramDeviceID: "7"}},
		{"wipe mobile", actionWipeMobile, "/JSSResource/mobiledevicecommands/command/EraseDevice/id/7", map[string]any{paramDeviceID: "7"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %q", r.Method)
				}
				if r.URL.Path != tc.path {
					t.Errorf("path = %q want %q", r.URL.Path, tc.path)
				}
			})
			res, err := p.Execute(context.Background(), creds, uuid.New(), tc.actionKey, "", tc.params)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !res.Success {
				t.Fatalf("not success: %+v", res)
			}
		})
	}
}

func TestJamf_RequiresDeviceID(t *testing.T) {
	p, creds := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {})
	_, err := p.Execute(context.Background(), creds, uuid.New(), actionLockComputer, "", nil)
	if err == nil || !strings.Contains(err.Error(), "device_id") {
		t.Fatalf("expected device_id error, got %v", err)
	}
}

func TestJamf_RealTokenEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/token" {
			if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
				t.Errorf("content-type = %q", got)
			}
			_, _ = w.Write([]byte(`{"access_token":"real-tok","expires_in":1800}`))
			return
		}
		if r.URL.Path == "/api/v1/auth" {
			if got := r.Header.Get("Authorization"); got != "Bearer real-tok" {
				t.Errorf("auth = %q", got)
			}
			return
		}
		t.Errorf("unexpected path %q", r.URL.Path)
	}))
	defer srv.Close()
	p := New()
	p.SetHTTPClient(srv.Client())
	creds := domain.Credentials{
		credBaseURL:      srv.URL,
		credClientID:     "client",
		credClientSecret: "secret",
	}
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}
