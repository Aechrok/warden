package rbac

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Checker resolves and matches permissions for an operator. The wildcard
// rules from permissions.go (Matches) apply: a stored grant of "holds:*"
// satisfies a check for "holds:read".
type Checker struct{}

// NewChecker constructs a Checker. Stateless; the pool is supplied on each
// call so the same instance can be used across requests.
func NewChecker() *Checker { return &Checker{} }

// HasPermission reports whether the user has been granted (directly or via
// wildcard) the requested permission. Returns false (with nil error) when
// the user has no roles assigned.
func (c *Checker) HasPermission(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, perm Permission) (bool, error) {
	if pool == nil {
		return false, errors.New("rbac: has permission: nil pool")
	}
	if userID == uuid.Nil {
		return false, nil
	}

	rows, err := pool.Query(ctx, `
		SELECT rp.permission
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
	`, userID)
	if err != nil {
		return false, fmt.Errorf("rbac: query permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var granted string
		if err := rows.Scan(&granted); err != nil {
			return false, fmt.Errorf("rbac: scan permission: %w", err)
		}
		if Matches(granted, perm.String()) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("rbac: rows: %w", err)
	}
	return false, nil
}

// GetPermissions returns the effective permission set for a user, deduped.
// Wildcards are returned literally — callers that need to display "all"
// effective permissions to a human should expand them against AllPermissions.
func (c *Checker) GetPermissions(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]Permission, error) {
	if pool == nil {
		return nil, errors.New("rbac: get permissions: nil pool")
	}
	if userID == uuid.Nil {
		return nil, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT rp.permission
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY rp.permission ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: query permissions: %w", err)
	}
	defer rows.Close()

	out := []Permission{}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, fmt.Errorf("rbac: scan permission: %w", err)
		}
		out = append(out, Permission(g))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: rows: %w", err)
	}
	return out, nil
}

// ExpandPermissions takes a possibly-wildcarded grant set and returns the
// flattened canonical permission list it implies. Useful for UI display
// (showing "you have N permissions") and for token scope checks.
func ExpandPermissions(grants []Permission) []Permission {
	expanded := map[Permission]struct{}{}
	for _, g := range grants {
		gs := g.String()
		for _, p := range AllPermissions() {
			if Matches(gs, p.String()) {
				expanded[p] = struct{}{}
			}
		}
	}
	out := make([]Permission, 0, len(expanded))
	for _, p := range AllPermissions() {
		if _, ok := expanded[p]; ok {
			out = append(out, p)
		}
	}
	return out
}

// GetRoles returns the list of role names assigned to a user, ordered by
// name. Empty slice (not nil) when the user has no roles.
func (c *Checker) GetRoles(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) ([]string, error) {
	if pool == nil {
		return nil, errors.New("rbac: get roles: nil pool")
	}
	rows, err := pool.Query(ctx, `
		SELECT r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.name ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("rbac: query roles: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("rbac: scan role: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rbac: rows: %w", err)
	}
	return out, nil
}

// AssignRole grants a role to a user. Idempotent. grantedBy may be uuid.Nil
// for system-driven assignments (e.g. SCIM provisioning).
func (c *Checker) AssignRole(ctx context.Context, pool *pgxpool.Pool, userID, roleID, grantedBy uuid.UUID) error {
	if pool == nil {
		return errors.New("rbac: assign role: nil pool")
	}
	var granter any
	if grantedBy != uuid.Nil {
		granter = grantedBy
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, granted_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID, roleID, granter)
	if err != nil {
		return fmt.Errorf("rbac: assign role: %w", err)
	}
	return nil
}

// RevokeRole removes a role assignment. Idempotent.
func (c *Checker) RevokeRole(ctx context.Context, pool *pgxpool.Pool, userID, roleID uuid.UUID) error {
	if pool == nil {
		return errors.New("rbac: revoke role: nil pool")
	}
	_, err := pool.Exec(ctx, `
		DELETE FROM user_roles
		WHERE user_id = $1 AND role_id = $2
	`, userID, roleID)
	if err != nil {
		return fmt.Errorf("rbac: revoke role: %w", err)
	}
	return nil
}

// RoleIDByName resolves a role name to its UUID. Returns uuid.Nil and a
// wrapped pgx.ErrNoRows if the role does not exist.
func RoleIDByName(ctx context.Context, pool *pgxpool.Pool, name string) (uuid.UUID, error) {
	if pool == nil {
		return uuid.Nil, errors.New("rbac: role lookup: nil pool")
	}
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, name).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("rbac: role %q: %w", name, err)
	}
	return id, nil
}
