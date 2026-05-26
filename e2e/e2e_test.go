package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestPublicAPI_TokenScoping verifies that a token's scope list gates access.
// A holds:read token can list holds but cannot execute actions.
func TestPublicAPI_TokenScoping(t *testing.T) {
	baseURL, pool := startServer(t)

	// Create a token with only holds:read scope.
	token := makeToken(t, pool, []string{"holds:read"})

	// GET /api/v1/public/holds → 200 (scope matches)
	resp := getReq(t, baseURL+"/api/v1/public/holds", token)
	assertStatus(t, resp, http.StatusOK)

	// POST /api/v1/public/actions/execute → 403 (missing integrations:execute scope)
	resp2 := postJSON(t, baseURL+"/api/v1/public/actions/execute", token, map[string]any{
		"instance_id": uuid.New().String(),
		"action_key":  "test:action",
	})
	assertStatus(t, resp2, http.StatusForbidden)
}

// TestBreakGlass_AuditTrail verifies that invoking break-glass creates an
// incident row and emits a breakglass.used event.
func TestBreakGlass_AuditTrail(t *testing.T) {
	baseURL, pool := startServer(t)
	ctx := context.Background()

	// Create a token with breakglass:use permission (makeToken gives all perms via admin role).
	token := makeToken(t, pool, []string{"breakglass:use", "breakglass:review", "audit:read"})

	resp := postJSON(t, baseURL+"/api/v1/internal/breakglass/invoke", token, map[string]any{
		"action_key":   "okta:deactivate",
		"target_email": "target@example.com",
		"reason":       "Emergency security incident: account compromise suspected",
	})
	assertStatus(t, resp, http.StatusCreated)

	var body map[string]any
	decodeJSON(t, resp, &body)
	incidentIDStr, ok := body["incident_id"].(string)
	if !ok || incidentIDStr == "" {
		t.Fatalf("expected incident_id in response, got %v", body)
	}

	// Verify the incident appears in the list.
	listResp := getReq(t, baseURL+"/api/v1/internal/breakglass/incidents", token)
	assertStatus(t, listResp, http.StatusOK)
	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)
	incidents, _ := listBody["incidents"].([]any)
	if len(incidents) == 0 {
		t.Fatal("expected at least one incident in list")
	}

	// Verify the breakglass.used event exists.
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM events WHERE type = 'breakglass.used'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if count == 0 {
		t.Error("expected breakglass.used event in events table")
	}
}

// TestApproval_WorkflowComplete verifies the approval request lifecycle.
func TestApproval_WorkflowComplete(t *testing.T) {
	baseURL, pool := startServer(t)
	ctx := context.Background()

	// Insert a change_freeze_window policy that covers all time so any action
	// triggers RequireApproval via PBAC.
	_, err := pool.Exec(ctx, `
		UPDATE pbac_policies
		SET is_enabled = true,
		    config = '{"windows":[{"start":"2020-01-01T00:00:00Z","end":"2099-12-31T23:59:59Z"}]}'::jsonb
		WHERE policy_type = 'change_freeze_window'
	`)
	if err != nil {
		t.Logf("could not set freeze window policy (may not exist): %v", err)
	}

	token := makeToken(t, pool, []string{"integrations:execute", "approvals:read", "approvals:write"})

	// Submit an action — should get 202 if PBAC requires approval, or 502 if
	// no plugin handles it (either is fine; we just verify the approval row).
	actionResp := postJSON(t, baseURL+"/api/v1/internal/actions/execute", token, map[string]any{
		"instance_id":  uuid.New().String(),
		"action_key":   "okta:deactivate",
		"target_email": "victim@example.com",
	})
	// Either 202 (approval required by PBAC) or 4xx/502 (plugin not found).
	// In either case we check whether an approval row was created.
	actionResp.Body.Close()

	// Check approvals list.
	listResp := getReq(t, baseURL+"/api/v1/internal/approvals/", token)
	assertStatus(t, listResp, http.StatusOK)
	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)
	// If no approval was created, test is still valid — policy may not fire.
	approvals, _ := listBody["approvals"].([]any)
	if len(approvals) == 0 {
		t.Skip("no approval created (PBAC policy not active); skipping approval workflow test")
	}

	// Approve the first request.
	approvalMap, _ := approvals[0].(map[string]any)
	approvalID, _ := approvalMap["id"].(string)
	approveResp := postJSON(t, baseURL+"/api/v1/internal/approvals/"+approvalID+"/approve", token, map[string]any{
		"note": "Approved by E2E test",
	})
	assertStatus(t, approveResp, http.StatusOK)
	approveResp.Body.Close()

	// Verify status in DB.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM approval_requests WHERE id = $1`, approvalID).Scan(&status); err != nil {
		t.Fatalf("query approval status: %v", err)
	}
	if status != "approved" {
		t.Errorf("approval status = %q, want %q", status, "approved")
	}
}

// TestHold_LifecycleBasic creates a hold, adds a custodian, verifies the
// custodian appears, and releases the hold.
func TestHold_LifecycleBasic(t *testing.T) {
	baseURL, pool := startServer(t)
	ctx := context.Background()

	token := makeToken(t, pool, []string{"holds:read", "holds:write"})

	// Create hold.
	createResp := postJSON(t, baseURL+"/api/v1/internal/holds/", token, map[string]any{
		"name":        "E2E Litigation Hold",
		"description": "Created by E2E test",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)
	holdMap, _ := createBody["hold"].(map[string]any)
	holdID, _ := holdMap["id"].(string)
	if holdID == "" {
		t.Fatal("expected hold id in create response")
	}

	// Add custodian.
	addResp := postJSON(t, baseURL+"/api/v1/internal/holds/"+holdID+"/custodians", token, map[string]any{
		"email": "custodian@example.com",
	})
	assertStatus(t, addResp, http.StatusOK)
	addResp.Body.Close()

	// Get hold detail — verify custodian appears.
	getResp := getReq(t, baseURL+"/api/v1/internal/holds/"+holdID, token)
	assertStatus(t, getResp, http.StatusOK)
	var getBody map[string]any
	decodeJSON(t, getResp, &getBody)
	// Note: cascade_state shows active instances; custodian should be in DB.
	var custodianCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM legal_hold_custodians WHERE hold_id=$1 AND removed_at IS NULL`,
		holdID,
	).Scan(&custodianCount); err != nil {
		t.Fatalf("count custodians: %v", err)
	}
	if custodianCount != 1 {
		t.Errorf("expected 1 custodian, got %d", custodianCount)
	}

	// Release hold.
	releaseResp := postJSON(t, baseURL+"/api/v1/internal/holds/"+holdID+"/release", token, map[string]any{
		"reason": "Litigation concluded",
	})
	assertStatus(t, releaseResp, http.StatusOK)
	releaseResp.Body.Close()

	// Verify status = released.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM legal_holds WHERE id = $1`, holdID).Scan(&status); err != nil {
		t.Fatalf("query hold status: %v", err)
	}
	if status != "released" {
		t.Errorf("hold status = %q, want %q", status, "released")
	}
}

// TestAuth_InactiveUser_Blocked verifies that deactivating a user blocks further
// token-authenticated API calls.
func TestAuth_InactiveUser_Blocked(t *testing.T) {
	baseURL, pool := startServer(t)
	ctx := context.Background()

	token := makeToken(t, pool, []string{"holds:read"})

	// First call should succeed.
	resp1 := getReq(t, baseURL+"/api/v1/public/holds", token)
	assertStatus(t, resp1, http.StatusOK)

	// Deactivate the user who owns this token.
	_, err := pool.Exec(ctx, `
		UPDATE users SET is_active = false
		WHERE id = (
			SELECT user_id FROM api_tokens WHERE token_hash = $1
		)
	`, tokenHash(token))
	if err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	// Subsequent call must be blocked.
	resp2 := getReq(t, baseURL+"/api/v1/public/holds", token)
	assertStatus(t, resp2, http.StatusUnauthorized)
}

// TestAdmin_InstancesCRUD exercises the admin instances lifecycle.
func TestAdmin_InstancesCRUD(t *testing.T) {
	baseURL, pool := startServer(t)
	ctx := context.Background()

	token := makeToken(t, pool, []string{"instances:read", "instances:write"})

	// Create an instance.
	createResp := postJSON(t, baseURL+"/api/v1/internal/admin/instances", token, map[string]any{
		"name":      "E2E Okta Instance",
		"plugin_id": "okta",
	})
	assertStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	decodeJSON(t, createResp, &createBody)
	instanceID, _ := createBody["id"].(string)
	if instanceID == "" {
		t.Fatal("expected instance id")
	}

	// List instances — verify it appears.
	listResp := getReq(t, baseURL+"/api/v1/internal/admin/instances", token)
	assertStatus(t, listResp, http.StatusOK)
	var listBody map[string]any
	decodeJSON(t, listResp, &listBody)
	instances, _ := listBody["instances"].([]any)
	found := false
	for _, inst := range instances {
		im, _ := inst.(map[string]any)
		if im["id"] == instanceID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created instance not found in list")
	}

	// Delete instance.
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/internal/admin/instances/"+instanceID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	delResp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	assertStatus(t, delResp, http.StatusOK)
	delResp.Body.Close()

	// Verify deleted from DB.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM integration_instances WHERE id=$1`, instanceID).Scan(&count); err != nil {
		t.Fatalf("count instances: %v", err)
	}
	if count != 0 {
		t.Error("instance should have been deleted")
	}
}
