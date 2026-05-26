package rbac

import "testing"

func TestMatches(t *testing.T) {
	cases := []struct {
		granted   string
		requested string
		want      bool
	}{
		{"*", "holds:read", true},
		{"*", "anything", true},
		{"holds:*", "holds:read", true},
		{"holds:*", "holds:write", true},
		{"holds:*", "audit:read", false},
		{"holds:read", "holds:read", true},
		{"holds:read", "holds:write", false},
		{"", "holds:read", false},
		{"holds:*", "", false},
		// Wildcard only matches at the segment level: "hold*" is not a
		// recognized form so it should not match by prefix.
		{"hold*", "holds:read", false},
	}
	for _, c := range cases {
		got := Matches(c.granted, c.requested)
		if got != c.want {
			t.Errorf("Matches(%q,%q) = %v, want %v", c.granted, c.requested, got, c.want)
		}
	}
}

func TestAllPermissionsKnown(t *testing.T) {
	perms := AllPermissions()
	if len(perms) != 28 {
		t.Fatalf("expected 28 canonical permissions, got %d", len(perms))
	}
	for _, p := range perms {
		if !IsKnown(p) {
			t.Errorf("IsKnown(%q) = false", p)
		}
	}
	if IsKnown("totally:fake") {
		t.Error("IsKnown should reject unknown permissions")
	}
}

func TestExpandPermissions(t *testing.T) {
	out := ExpandPermissions([]Permission{"holds:*"})
	if len(out) != 2 {
		t.Fatalf("expected 2 expansions for holds:*, got %d", len(out))
	}
	wantSet := map[Permission]bool{PermHoldsRead: true, PermHoldsWrite: true}
	for _, p := range out {
		if !wantSet[p] {
			t.Errorf("unexpected permission expansion: %q", p)
		}
	}

	out = ExpandPermissions([]Permission{"*"})
	if len(out) != len(AllPermissions()) {
		t.Errorf("expected * to expand to every permission, got %d", len(out))
	}
}
