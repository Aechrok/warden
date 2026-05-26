package rbac

import "testing"

func TestBuiltinRolesShape(t *testing.T) {
	if len(BuiltinRoles) != 6 {
		t.Fatalf("expected 6 built-in roles, got %d", len(BuiltinRoles))
	}
	names := map[string]bool{}
	for _, r := range BuiltinRoles {
		if r.Name == "" {
			t.Error("built-in role missing name")
		}
		if r.Description == "" {
			t.Errorf("role %q missing description", r.Name)
		}
		if len(r.Permissions) == 0 {
			t.Errorf("role %q has no permissions", r.Name)
		}
		if names[r.Name] {
			t.Errorf("duplicate role name %q", r.Name)
		}
		names[r.Name] = true
	}
	for _, want := range []string{
		RoleAdmin, RoleOperator, RoleAuditor,
		RoleLegalOperator, RoleReadOnly, RoleApprover,
	} {
		if !names[want] {
			t.Errorf("missing built-in role %q", want)
		}
	}
}

func TestAdminHasWildcard(t *testing.T) {
	admin := BuiltinRoleByName(RoleAdmin)
	if admin == nil {
		t.Fatal("admin role missing")
	}
	if len(admin.Permissions) != 1 || admin.Permissions[0] != "*" {
		t.Errorf("admin permissions = %v, want [*]", admin.Permissions)
	}
}

func TestBuiltinPermissionsResolve(t *testing.T) {
	// Every permission grant on every built-in role must either be a
	// wildcard or a known canonical permission. Otherwise the seeder will
	// persist a string that nothing in the codebase can validate against.
	for _, r := range BuiltinRoles {
		for _, p := range r.Permissions {
			if p == "*" {
				continue
			}
			// Wildcard form like "holds:*" must expand to at least one
			// known canonical permission.
			expanded := ExpandPermissions([]Permission{Permission(p)})
			if len(expanded) == 0 {
				t.Errorf("role %q permission %q expands to nothing", r.Name, p)
			}
		}
	}
}
