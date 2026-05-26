package rbac_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/aechrok/warden/internal/rbac"
	"github.com/aechrok/warden/internal/testutil"
)

func TestHasPermission_granted(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	checker := rbac.NewChecker()

	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,name) VALUES ('rbac_granted@example.com','RBAC User') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO roles (name,description) VALUES ('rbac_test_role','Test') RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id,permission) VALUES ($1,'holds:read')`, roleID); err != nil {
		t.Fatalf("insert permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id,role_id) VALUES ($1,$2)`, userID, roleID); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	ok, err := checker.HasPermission(ctx, pool, userID, rbac.PermHoldsRead)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !ok {
		t.Error("expected permission to be granted")
	}
}

func TestHasPermission_denied(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	checker := rbac.NewChecker()

	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,name) VALUES ('rbac_denied@example.com','RBAC Denied') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ok, err := checker.HasPermission(ctx, pool, userID, rbac.PermHoldsRead)
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if ok {
		t.Error("expected permission to be denied for user with no roles")
	}
}

func TestGetPermissions_wildcard(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	checker := rbac.NewChecker()

	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,name) VALUES ('rbac_wildcard@example.com','Wildcard User') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO roles (name,description) VALUES ('wildcard_role','Wildcard Test') RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id,permission) VALUES ($1,'holds:*')`, roleID); err != nil {
		t.Fatalf("insert wildcard permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_roles (user_id,role_id) VALUES ($1,$2)`, userID, roleID); err != nil {
		t.Fatalf("assign role: %v", err)
	}

	grants, err := checker.GetPermissions(ctx, pool, userID)
	if err != nil {
		t.Fatalf("GetPermissions: %v", err)
	}
	expanded := rbac.ExpandPermissions(grants)

	hasRead, hasWrite := false, false
	for _, p := range expanded {
		if p == rbac.PermHoldsRead {
			hasRead = true
		}
		if p == rbac.PermHoldsWrite {
			hasWrite = true
		}
	}
	if !hasRead {
		t.Error("ExpandPermissions: expected holds:read from holds:*")
	}
	if !hasWrite {
		t.Error("ExpandPermissions: expected holds:write from holds:*")
	}
}

func TestAssignRevoke_idempotent(t *testing.T) {
	pool := testutil.NewTestPool(t)
	ctx := context.Background()
	checker := rbac.NewChecker()

	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email,name) VALUES ('rbac_idempotent@example.com','Idempotent User') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO roles (name,description) VALUES ('idempotent_role','Idempotent Test') RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("insert role: %v", err)
	}

	// Assign the same role twice — should be idempotent.
	if err := checker.AssignRole(ctx, pool, userID, roleID, uuid.Nil); err != nil {
		t.Fatalf("AssignRole (1st): %v", err)
	}
	if err := checker.AssignRole(ctx, pool, userID, roleID, uuid.Nil); err != nil {
		t.Fatalf("AssignRole (2nd, idempotent): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE user_id=$1 AND role_id=$2`, userID, roleID).Scan(&count); err != nil {
		t.Fatalf("count user_roles: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 user_roles row after double assign, got %d", count)
	}

	// Revoke and verify.
	if err := checker.RevokeRole(ctx, pool, userID, roleID); err != nil {
		t.Fatalf("RevokeRole: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE user_id=$1 AND role_id=$2`, userID, roleID).Scan(&count); err != nil {
		t.Fatalf("count after revoke: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 user_roles rows after revoke, got %d", count)
	}
}
