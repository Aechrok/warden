// Package rbac implements Warden's role-based access control: the canonical
// permission catalog, built-in role definitions, an idempotent seeder, and a
// runtime permission checker with wildcard matching.
package rbac

import "strings"

// Permission is a canonical permission string of the form "resource:action".
// All checks are done against the constants defined in this file so typos
// fail at compile time.
type Permission string

// String renders the permission for storage and logging.
func (p Permission) String() string { return string(p) }

// Canonical permission set. These 26 values are the only permissions Warden
// recognizes; adding one requires a code change and is intentional.
const (
	PermUsersRead          Permission = "users:read"
	PermUsersWrite         Permission = "users:write"
	PermAuditRead          Permission = "audit:read"
	PermIdentitiesRead     Permission = "identities:read"
	PermIdentitiesWrite    Permission = "identities:write"
	PermDevicesRead        Permission = "devices:read"
	PermIntegrationsRead   Permission = "integrations:read"
	PermIntegrationsExec   Permission = "integrations:execute"
	PermInstancesRead      Permission = "instances:read"
	PermInstancesWrite     Permission = "instances:write"
	PermHoldsRead          Permission = "holds:read"
	PermHoldsWrite         Permission = "holds:write"
	PermHoldTemplatesRead  Permission = "hold_templates:read"
	PermHoldTemplatesWrite Permission = "hold_templates:write"
	PermRolesRead          Permission = "roles:read"
	PermRolesWrite         Permission = "roles:write"
	PermTokensRead         Permission = "tokens:read"
	PermTokensWrite        Permission = "tokens:write"
	PermApprovalsRead      Permission = "approvals:read"
	PermApprovalsWrite     Permission = "approvals:write"
	PermBreakGlassUse      Permission = "breakglass:use"
	PermBreakGlassReview   Permission = "breakglass:review"
	PermPBACRead           Permission = "pbac_policies:read"
	PermPBACWrite          Permission = "pbac_policies:write"
	PermVIPRead            Permission = "vip_identities:read"
	PermVIPWrite           Permission = "vip_identities:write"
	PermAssistantUse       Permission = "assistant:use"
	PermSCIMAdmin          Permission = "scim:admin"
)

// AllPermissions returns the full canonical permission catalog in declaration
// order. Used by the seeder, the admin UI, and validation paths that need
// to reject unknown permission strings.
func AllPermissions() []Permission {
	return []Permission{
		PermUsersRead,
		PermUsersWrite,
		PermAuditRead,
		PermIdentitiesRead,
		PermIdentitiesWrite,
		PermDevicesRead,
		PermIntegrationsRead,
		PermIntegrationsExec,
		PermInstancesRead,
		PermInstancesWrite,
		PermHoldsRead,
		PermHoldsWrite,
		PermHoldTemplatesRead,
		PermHoldTemplatesWrite,
		PermRolesRead,
		PermRolesWrite,
		PermTokensRead,
		PermTokensWrite,
		PermApprovalsRead,
		PermApprovalsWrite,
		PermBreakGlassUse,
		PermBreakGlassReview,
		PermPBACRead,
		PermPBACWrite,
		PermVIPRead,
		PermVIPWrite,
		PermAssistantUse,
		PermSCIMAdmin,
	}
}

// IsKnown reports whether p is one of the canonical permissions. Used to
// guard SCIM and YAML imports against silent typos.
func IsKnown(p Permission) bool {
	for _, known := range AllPermissions() {
		if known == p {
			return true
		}
	}
	return false
}

// Matches implements the wildcard match semantics used by the checker.
//
//	"*"          matches every permission
//	"holds:*"    matches "holds:read" and "holds:write"
//	"holds:read" matches only itself
//
// granted is the permission string stored on a role (which may contain a
// wildcard), requested is the concrete permission being checked.
func Matches(granted, requested string) bool {
	if granted == "" {
		return false
	}
	if granted == "*" {
		return true
	}
	if granted == requested {
		return true
	}
	if strings.HasSuffix(granted, ":*") {
		prefix := strings.TrimSuffix(granted, "*") // keeps the trailing ":"
		return strings.HasPrefix(requested, prefix)
	}
	return false
}
