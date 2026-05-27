package rbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// BootstrapFirstAdmin assigns the admin role to userID if — and only if —
// no user currently holds the admin role. This is called on every successful
// login so the very first operator to authenticate becomes the initial admin.
// It is a no-op once any admin assignment exists.
func BootstrapFirstAdmin(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	var count int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE r.name = 'admin'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("rbac: bootstrap admin check: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = 'admin'
		ON CONFLICT DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("rbac: bootstrap admin assign: %w", err)
	}
	return nil
}
