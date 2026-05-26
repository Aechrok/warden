package rbac

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Built-in role names. These are the names inserted by the Seeder; admins
// may create additional custom roles via the API.
const (
	RoleAdmin         = "admin"
	RoleOperator      = "operator"
	RoleAuditor       = "auditor"
	RoleLegalOperator = "legal_operator"
	RoleReadOnly      = "read_only"
	RoleApprover      = "approver"
)

// BuiltinRole is a role definition used by the seeder. Permissions are
// stored as strings (not Permission) because they may include wildcards
// like "holds:*" which are not in the canonical AllPermissions() set but
// are perfectly valid grant entries.
type BuiltinRole struct {
	Name        string
	Description string
	Permissions []string
}

// BuiltinRoles is the canonical built-in role catalog. The seeder upserts
// these on every boot so they cannot drift out of sync with the code.
var BuiltinRoles = []BuiltinRole{
	{
		Name:        RoleAdmin,
		Description: "Full administrative access to every Warden subsystem.",
		Permissions: []string{"*"},
	},
	{
		Name:        RoleOperator,
		Description: "Day-to-day operator: identity lookups, action execution, holds, tokens.",
		Permissions: []string{
			"identities:*",
			"devices:read",
			"integrations:*",
			"holds:*",
			"hold_templates:read",
			"approvals:*",
			"breakglass:use",
			"tokens:*",
			"audit:read",
			"assistant:use",
		},
	},
	{
		Name:        RoleAuditor,
		Description: "Read-only audit access across the platform.",
		Permissions: []string{
			"audit:read",
			"identities:read",
			"holds:read",
			"roles:read",
			"tokens:read",
			"devices:read",
			"instances:read",
		},
	},
	{
		Name:        RoleLegalOperator,
		Description: "Legal team: holds and hold templates with identity / audit visibility.",
		Permissions: []string{
			"holds:*",
			"hold_templates:*",
			"identities:read",
			"audit:read",
		},
	},
	{
		Name:        RoleReadOnly,
		Description: "Minimal read access across identities, devices, holds, audit, and tokens.",
		Permissions: []string{
			"identities:read",
			"devices:read",
			"holds:read",
			"audit:read",
			"tokens:read",
		},
	},
	{
		Name:        RoleApprover,
		Description: "Reviews destructive actions and break-glass incidents.",
		Permissions: []string{
			"approvals:*",
			"breakglass:review",
			"audit:read",
		},
	},
}

// BuiltinRoleByName returns the BuiltinRole definition for the given name,
// or nil if no such built-in role exists.
func BuiltinRoleByName(name string) *BuiltinRole {
	for i := range BuiltinRoles {
		if BuiltinRoles[i].Name == name {
			return &BuiltinRoles[i]
		}
	}
	return nil
}

// Seeder inserts (or updates) the built-in roles and their permissions. Safe
// to call on every server start; the operations are idempotent.
type Seeder struct{}

// NewSeeder constructs a Seeder.
func NewSeeder() *Seeder { return &Seeder{} }

// Seed writes every built-in role to the database. The operation runs in a
// single transaction so a partially-seeded state is never visible.
//
// For each built-in role:
//   - INSERT ... ON CONFLICT(name) DO UPDATE bumps description and returns the row id
//   - Existing role_permissions rows for that role are deleted and replaced
//     with the canonical set, so demotions (removing a permission from the
//     built-in definition) actually take effect on the next boot.
func (s *Seeder) Seed(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("rbac: seeder: nil pool")
	}
	return runTx(ctx, pool, func(tx pgx.Tx) error {
		for _, role := range BuiltinRoles {
			if err := seedRole(ctx, tx, role); err != nil {
				return fmt.Errorf("rbac: seed role %q: %w", role.Name, err)
			}
		}
		return nil
	})
}

func seedRole(ctx context.Context, tx pgx.Tx, r BuiltinRole) error {
	var roleID string
	err := tx.QueryRow(ctx, `
		INSERT INTO roles (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE
		  SET description = EXCLUDED.description
		RETURNING id::text
	`, r.Name, r.Description).Scan(&roleID)
	if err != nil {
		return fmt.Errorf("upsert role: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, roleID); err != nil {
		return fmt.Errorf("clear permissions: %w", err)
	}
	for _, perm := range r.Permissions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission)
			VALUES ($1::uuid, $2)
			ON CONFLICT (role_id, permission) DO NOTHING
		`, roleID, perm); err != nil {
			return fmt.Errorf("insert permission %q: %w", perm, err)
		}
	}
	return nil
}

func runTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
