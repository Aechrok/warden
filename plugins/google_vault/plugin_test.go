package google_vault

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
)

type vaultFake struct {
	mu      sync.Mutex
	matters map[string]string // name -> id
	holds   map[string]map[string]map[string]any // matterID -> holdID -> hold body
	next    int
}

func newFake() *vaultFake {
	return &vaultFake{
		matters: map[string]string{},
		holds:   map[string]map[string]map[string]any{},
	}
}

func (f *vaultFake) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		switch {
		case r.Method == http.MethodGet && p == "/v1/matters":
			out := []map[string]any{}
			for name, id := range f.matters {
				out = append(out, map[string]any{"matterId": id, "name": name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"matters": out})
		case r.Method == http.MethodPost && p == "/v1/matters":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			name, _ := body["name"].(string)
			f.next++
			id := uuid.New().String()
			f.matters[name] = id
			f.holds[id] = map[string]map[string]any{}
			_ = json.NewEncoder(w).Encode(map[string]any{"matterId": id, "name": name})
		case r.Method == http.MethodGet && strings.HasSuffix(p, "/holds"):
			parts := strings.Split(p, "/")
			matterID := parts[3]
			out := []map[string]any{}
			for hid, h := range f.holds[matterID] {
				out = append(out, map[string]any{"holdId": hid, "name": h["name"]})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"holds": out})
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/holds"):
			parts := strings.Split(p, "/")
			matterID := parts[3]
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			hid := uuid.New().String()
			f.holds[matterID][hid] = body
			_ = json.NewEncoder(w).Encode(map[string]any{"holdId": hid, "name": body["name"]})
		case r.Method == http.MethodDelete && strings.Contains(p, "/holds/"):
			parts := strings.Split(p, "/")
			matterID := parts[3]
			holdID := parts[5]
			delete(f.holds[matterID], holdID)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, p)
			http.Error(w, "not implemented", http.StatusNotImplemented)
		}
	})
}

func newTestPlugin(t *testing.T, fake *vaultFake) (*Plugin, domain.Credentials) {
	t.Helper()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)
	p := New()
	p.client = srv.Client()
	p.vaultBaseURL = srv.URL
	p.tokenFn = func(ctx context.Context, sa []byte, admin string, scopes []string) (string, error) {
		return "tok", nil
	}
	return p, domain.Credentials{
		credServiceAccount: "{}",
		credAdminEmail:     "admin@example.com",
	}
}

func TestVault_PlaceAndRemoveHold_Idempotent(t *testing.T) {
	fake := newFake()
	p, creds := newTestPlugin(t, fake)
	instanceID := uuid.New()
	ctx := context.Background()

	res, err := p.PlaceHold(ctx, creds, instanceID, "ada@example.com")
	if err != nil {
		t.Fatalf("PlaceHold: %v", err)
	}
	if !res.Success {
		t.Fatalf("PlaceHold not success: %+v", res)
	}
	if len(fake.matters) != 1 {
		t.Fatalf("expected 1 matter, got %d", len(fake.matters))
	}
	matterID := ""
	for _, id := range fake.matters {
		matterID = id
	}
	if len(fake.holds[matterID]) != 3 {
		t.Fatalf("expected 3 holds, got %d", len(fake.holds[matterID]))
	}

	// Second PlaceHold call must be idempotent: no duplicate holds.
	if _, err := p.PlaceHold(ctx, creds, instanceID, "ada@example.com"); err != nil {
		t.Fatalf("idempotent PlaceHold: %v", err)
	}
	if len(fake.holds[matterID]) != 3 {
		t.Fatalf("after idempotent place, got %d holds", len(fake.holds[matterID]))
	}

	// RemoveHold deletes all corpora.
	if _, err := p.RemoveHold(ctx, creds, instanceID, "ada@example.com"); err != nil {
		t.Fatalf("RemoveHold: %v", err)
	}
	if len(fake.holds[matterID]) != 0 {
		t.Fatalf("expected 0 holds after remove, got %d", len(fake.holds[matterID]))
	}

	// Removing again is a no-op.
	if _, err := p.RemoveHold(ctx, creds, instanceID, "ada@example.com"); err != nil {
		t.Fatalf("idempotent RemoveHold: %v", err)
	}
}

func TestVault_HealthCheck(t *testing.T) {
	fake := newFake()
	p, creds := newTestPlugin(t, fake)
	if err := p.HealthCheck(context.Background(), creds); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}
