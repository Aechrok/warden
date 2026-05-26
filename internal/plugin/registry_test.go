package plugin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/domain"
)

type stubPlugin struct {
	id string
}

func (s *stubPlugin) ID() string                                                { return s.id }
func (s *stubPlugin) Name() string                                              { return s.id }
func (s *stubPlugin) Description() string                                       { return "" }
func (s *stubPlugin) CredentialSchema() []domain.CredentialField                { return nil }
func (s *stubPlugin) HealthCheck(ctx context.Context, c domain.Credentials) error { return nil }

func TestRegistryAddGetAll(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&stubPlugin{id: "b"})
	reg.Add(&stubPlugin{id: "a"})
	if _, ok := reg.Get("a"); !ok {
		t.Fatal("Get(a) missing")
	}
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
	all := reg.All()
	if len(all) != 2 || all[0].ID() != "a" || all[1].ID() != "b" {
		t.Fatalf("All() not sorted: %v", all)
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&stubPlugin{id: "dup"})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate id")
		}
	}()
	reg.Add(&stubPlugin{id: "dup"})
}

func TestEnvPrefix(t *testing.T) {
	cases := map[string]string{
		"okta-prod":     "OKTA_PROD",
		"google_vault":  "GOOGLE_VAULT",
		"slack":         "SLACK",
		"My Workspace!": "MY_WORKSPACE_",
		"123abc":        "123ABC",
	}
	for in, want := range cases {
		if got := envPrefix(in); got != want {
			t.Errorf("envPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// Confirm that the uuid import is exercised so this file compiles even if
// future edits remove its uses from the registry surface tests.
var _ = uuid.Nil
