// Package internal implements Warden's session-authenticated internal API.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/api/middleware"
	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/breakglass"
	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/legalhold"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/internal/rbac"
	"github.com/aechrok/warden/internal/store"
)

// writeJSON encodes v as JSON and writes it with status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes {"error":"<msg>"} with status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Deps bundles all service dependencies for internal API handlers.
type Deps struct {
	Pool       *pgxpool.Pool
	Sessions   *auth.SessionStore
	Users      *auth.UserStore
	Checker    *rbac.Checker
	Dispatcher *plugin.Dispatcher
	HoldSvc    *legalhold.Service
	BreakGlass *breakglass.Service
	EventStore *store.EventStore
	Logger     *zap.Logger
	Secure     bool
}

// -----------------------------------------------------------------------
// Auth helpers
// -----------------------------------------------------------------------

// Logout deletes the session and clears the session cookie.
// POST /api/v1/internal/auth/logout
func (d *Deps) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = d.Sessions.Delete(r.Context(), d.Pool, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   d.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Me returns the current user's profile, permissions, and roles.
// GET /api/v1/internal/me
func (d *Deps) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms, err := d.Checker.GetPermissions(r.Context(), d.Pool, user.ID)
	if err != nil {
		d.Logger.Error("me: get permissions", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	roles, err := d.Checker.GetRoles(r.Context(), d.Pool, user.ID)
	if err != nil {
		d.Logger.Error("me: get roles", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        user,
		"permissions": perms,
		"roles":       roles,
	})
}

// -----------------------------------------------------------------------
// Identities
// -----------------------------------------------------------------------

// SearchIdentities resolves an email against a specific integration instance.
// GET /api/v1/internal/identities/search?email=&instance_id=
func (d *Deps) SearchIdentities(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}
	instanceIDStr := strings.TrimSpace(r.URL.Query().Get("instance_id"))
	if instanceIDStr == "" {
		writeError(w, http.StatusBadRequest, "instance_id required")
		return
	}
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance_id")
		return
	}
	identity, err := d.Dispatcher.GetIdentity(r.Context(), instanceID, email)
	if err != nil {
		d.Logger.Error("search identities", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": identity})
}

// RefreshIdentityCache re-runs the identity lookup and updates identity_cache.
// POST /api/v1/internal/identities/cache/refresh
func (d *Deps) RefreshIdentityCache(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	instanceID, err := uuid.Parse(body.InstanceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance_id")
		return
	}
	identity, err := d.Dispatcher.GetIdentity(r.Context(), instanceID, body.Email)
	if err != nil {
		d.Logger.Error("refresh identity cache", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	dataJSON, _ := json.Marshal(identity.Data)
	_, _ = d.Pool.Exec(r.Context(), `
		INSERT INTO identity_cache (email, instance_id, data, fetched_at)
		VALUES ($1, $2, $3::jsonb, now())
		ON CONFLICT (email, instance_id) DO UPDATE
		  SET data = EXCLUDED.data, fetched_at = now()
	`, strings.ToLower(body.Email), instanceID, string(dataJSON))
	writeJSON(w, http.StatusOK, map[string]any{"identity": identity})
}

// -----------------------------------------------------------------------
// Actions
// -----------------------------------------------------------------------

// ListActions returns available actions per active integration instance.
// GET /api/v1/internal/actions/
func (d *Deps) ListActions(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, name, plugin_id FROM integration_instances WHERE is_active = true ORDER BY name ASC
	`)
	if err != nil {
		d.Logger.Error("list actions: query instances", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type instanceActions struct {
		InstanceID   string                    `json:"instance_id"`
		InstanceName string                    `json:"instance_name"`
		PluginID     string                    `json:"plugin_id"`
		Actions      []domain.ActionDefinition `json:"actions"`
	}
	var result []instanceActions
	for rows.Next() {
		var id uuid.UUID
		var name, pluginID string
		if err := rows.Scan(&id, &name, &pluginID); err != nil {
			continue
		}
		p, ok := plugin.Get(pluginID)
		if !ok {
			continue
		}
		exec, ok := p.(domain.ActionExecutor)
		if !ok {
			continue
		}
		result = append(result, instanceActions{
			InstanceID:   id.String(),
			InstanceName: name,
			PluginID:     pluginID,
			Actions:      exec.Actions(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": result})
}

// ExecuteAction runs an action against an integration instance.
// POST /api/v1/internal/actions/execute
// RBAC gate: integrations:execute  PBAC gate: applied before this handler.
func (d *Deps) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InstanceID  string         `json:"instance_id"`
		ActionKey   string         `json:"action_key"`
		TargetEmail string         `json:"target_email"`
		Params      map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	instanceID, err := uuid.Parse(body.InstanceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance_id")
		return
	}

	user, _ := middleware.UserFromCtx(r.Context())

	result, execErr := d.Dispatcher.Execute(r.Context(), instanceID, body.ActionKey, body.TargetEmail, body.Params)
	if execErr != nil {
		d.Logger.Error("execute action: dispatch", zap.Error(execErr))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("action failed: %s", execErr.Error()))
		return
	}

	// Emit an action.executed event inside a transaction.
	if user != nil {
		go func() {
			tx, err := d.Pool.Begin(context.Background())
			if err != nil {
				return
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			actor := actorFromUser(user)
			d.appendActionEvent(context.Background(), tx, instanceID, body.ActionKey, body.TargetEmail, body.Params, actor)
			_ = tx.Commit(context.Background())
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (d *Deps) appendActionEvent(ctx context.Context, tx pgx.Tx, instanceID uuid.UUID, actionKey, targetEmail string, params map[string]any, actor domain.Actor) {
	version, err := d.EventStore.NextVersion(ctx, tx, domain.AggregateAction, instanceID)
	if err != nil {
		return
	}
	var actorID *uuid.UUID
	if actor.ID != uuid.Nil {
		id := actor.ID
		actorID = &id
	}
	evt := domain.Event{
		AggregateType: domain.AggregateAction,
		AggregateID:   instanceID,
		Version:       version,
		Type:          domain.EventActionExecuted,
		Payload: map[string]any{
			"action_key":   actionKey,
			"target_email": targetEmail,
			"params":       params,
		},
		ActorID:   actorID,
		ActorType: actor.Type,
	}
	_, _ = d.EventStore.Append(ctx, tx, evt)
}

// -----------------------------------------------------------------------
// Holds
// -----------------------------------------------------------------------

// CreateHold opens a new legal hold.
// POST /api/v1/internal/holds/
func (d *Deps) CreateHold(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		TemplateID  *string `json:"template_id,omitempty"`
		ExpiresAt   *string `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	var templateID *uuid.UUID
	if body.TemplateID != nil {
		id, err := uuid.Parse(*body.TemplateID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid template_id")
			return
		}
		templateID = &id
	}

	var expiresAt *time.Time
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expires_at (RFC3339 required)")
			return
		}
		expiresAt = &t
	}

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	hold, err := d.HoldSvc.CreateHold(r.Context(), tx, legalhold.CreateHoldParams{
		Name:        body.Name,
		Description: body.Description,
		TemplateID:  templateID,
		ExpiresAt:   expiresAt,
		Actor:       actor,
	})
	if err != nil {
		d.Logger.Error("create hold", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "could not create hold")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"hold": hold})
}

// ListHolds lists holds, optionally filtered by status.
// GET /api/v1/internal/holds/?status=
func (d *Deps) ListHolds(w http.ResponseWriter, r *http.Request) {
	var filter legalhold.ListHoldsFilter
	if s := r.URL.Query().Get("status"); s != "" {
		hs := domain.HoldStatus(s)
		filter.Status = &hs
	}
	holds, err := d.HoldSvc.ListHolds(r.Context(), d.Pool, filter)
	if err != nil {
		d.Logger.Error("list holds", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holds": holds})
}

// GetHold returns a hold with its cascade state rows.
// GET /api/v1/internal/holds/:id
func (d *Deps) GetHold(w http.ResponseWriter, r *http.Request, holdIDStr string) {
	holdID, err := uuid.Parse(holdIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hold id")
		return
	}
	hold, err := d.HoldSvc.GetHold(r.Context(), d.Pool, holdID)
	if err != nil {
		writeError(w, http.StatusNotFound, "hold not found")
		return
	}

	csRows, _ := d.Pool.Query(r.Context(), `
		SELECT id, hold_id, custodian_email, instance_id, status, last_error, attempts, completed_at, created_at, updated_at
		FROM cascade_state WHERE hold_id = $1 ORDER BY created_at ASC
	`, holdID)
	var cascadeStates []domain.CascadeState
	if csRows != nil {
		defer csRows.Close()
		for csRows.Next() {
			var cs domain.CascadeState
			_ = csRows.Scan(&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID, &cs.Status, &cs.LastError, &cs.Attempts, &cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt)
			cascadeStates = append(cascadeStates, cs)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"hold": hold, "cascade_state": cascadeStates})
}

// AddCustodian adds a custodian to an active hold.
// POST /api/v1/internal/holds/:id/custodians
func (d *Deps) AddCustodian(w http.ResponseWriter, r *http.Request, holdIDStr string) {
	holdID, err := uuid.Parse(holdIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hold id")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := d.HoldSvc.AddCustodian(r.Context(), tx, holdID, body.Email, actor); err != nil {
		d.Logger.Error("add custodian", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// RemoveCustodian soft-deletes a custodian from a hold.
// DELETE /api/v1/internal/holds/:id/custodians/:custodianId
func (d *Deps) RemoveCustodian(w http.ResponseWriter, r *http.Request, holdIDStr, custodianIDStr string) {
	holdID, err := uuid.Parse(holdIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hold id")
		return
	}
	custodianID, err := uuid.Parse(custodianIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid custodian id")
		return
	}

	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := d.HoldSvc.RemoveCustodian(r.Context(), tx, holdID, custodianID, actor); err != nil {
		d.Logger.Error("remove custodian", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ReleaseHold releases an active hold.
// POST /api/v1/internal/holds/:id/release
func (d *Deps) ReleaseHold(w http.ResponseWriter, r *http.Request, holdIDStr string) {
	holdID, err := uuid.Parse(holdIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid hold id")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := d.HoldSvc.ReleaseHold(r.Context(), tx, holdID, body.Reason, actor); err != nil {
		d.Logger.Error("release hold", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}

// -----------------------------------------------------------------------
// Audit
// -----------------------------------------------------------------------

// ListAuditEvents queries the events table.
// GET /api/v1/internal/audit/events?aggregate_type=&aggregate_id=&since=&limit=
func (d *Deps) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	aggType := q.Get("aggregate_type")
	aggID := q.Get("aggregate_id")
	since := q.Get("since")
	limit := 100
	if ls := q.Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	query := `SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at FROM events WHERE 1=1`
	args := []any{}
	idx := 1

	if aggType != "" {
		query += fmt.Sprintf(" AND aggregate_type = $%d", idx)
		args = append(args, aggType)
		idx++
	}
	if aggID != "" {
		id, err := uuid.Parse(aggID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid aggregate_id")
			return
		}
		query += fmt.Sprintf(" AND aggregate_id = $%d", idx)
		args = append(args, id)
		idx++
	}
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since (RFC3339 required)")
			return
		}
		query += fmt.Sprintf(" AND created_at > $%d", idx)
		args = append(args, t)
		idx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := d.Pool.Query(r.Context(), query, args...)
	if err != nil {
		d.Logger.Error("list audit events", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	events := scanEvents(rows)
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// ExportAuditEvents streams events as JSON or CSV.
// GET /api/v1/internal/audit/export?format=json|csv
func (d *Deps) ExportAuditEvents(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at
		FROM events ORDER BY created_at ASC
	`)
	if err != nil {
		d.Logger.Error("export audit events", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit_events.csv")
		_, _ = w.Write([]byte("id,aggregate_type,aggregate_id,version,type,actor_id,actor_type,created_at\n"))
		for rows.Next() {
			e, ok := scanOneEvent(rows)
			if !ok {
				continue
			}
			actorIDStr := ""
			if e.ActorID != nil {
				actorIDStr = e.ActorID.String()
			}
			_, _ = fmt.Fprintf(w, "%s,%s,%s,%d,%s,%s,%s,%s\n",
				e.ID, e.AggregateType, e.AggregateID, e.Version, e.Type,
				actorIDStr, e.ActorType, e.CreatedAt.Format(time.RFC3339),
			)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_, _ = w.Write([]byte(`{"events":[`))
	first := true
	for rows.Next() {
		e, ok := scanOneEvent(rows)
		if !ok {
			continue
		}
		if !first {
			_, _ = w.Write([]byte(","))
		}
		_ = enc.Encode(e)
		first = false
	}
	_, _ = w.Write([]byte("]}\n"))
}

// -----------------------------------------------------------------------
// Approvals
// -----------------------------------------------------------------------

// ListApprovals returns pending approval requests.
// GET /api/v1/internal/approvals/
func (d *Deps) ListApprovals(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, requester_id, action_key, instance_id, target_email, params, reason, status,
		       reviewer_id, review_note, expires_at, reviewed_at, created_at
		FROM approval_requests WHERE status = 'pending' ORDER BY created_at ASC
	`)
	if err != nil {
		d.Logger.Error("list approvals", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()
	approvals := scanApprovals(rows)
	writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
}

// ApproveRequest approves a pending approval request.
// POST /api/v1/internal/approvals/:id/approve
func (d *Deps) ApproveRequest(w http.ResponseWriter, r *http.Request, approvalIDStr string) {
	d.decideApproval(w, r, approvalIDStr, "approved")
}

// RejectRequest rejects a pending approval request.
// POST /api/v1/internal/approvals/:id/reject
func (d *Deps) RejectRequest(w http.ResponseWriter, r *http.Request, approvalIDStr string) {
	d.decideApproval(w, r, approvalIDStr, "rejected")
}

func (d *Deps) decideApproval(w http.ResponseWriter, r *http.Request, approvalIDStr, newStatus string) {
	approvalID, err := uuid.Parse(approvalIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approval id")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	var reviewerID *uuid.UUID
	if u, ok := middleware.UserFromCtx(r.Context()); ok {
		reviewerID = &u.ID
	}

	_, err = d.Pool.Exec(r.Context(), `
		UPDATE approval_requests
		SET status = $2, reviewer_id = $3, review_note = $4, reviewed_at = now()
		WHERE id = $1 AND status = 'pending'
	`, approvalID, newStatus, reviewerID, body.Note)
	if err != nil {
		d.Logger.Error("decide approval", zap.String("status", newStatus), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": newStatus})
}

// -----------------------------------------------------------------------
// Break-glass
// -----------------------------------------------------------------------

// InvokeBreakGlass records a break-glass incident.
// POST /api/v1/internal/breakglass/invoke
func (d *Deps) InvokeBreakGlass(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ActionKey   string  `json:"action_key"`
		InstanceID  *string `json:"instance_id"`
		TargetEmail string  `json:"target_email"`
		Reason      string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	req := breakglass.InvokeRequest{
		ActionKey:   body.ActionKey,
		TargetEmail: body.TargetEmail,
		Reason:      body.Reason,
	}
	if body.InstanceID != nil {
		id, err := uuid.Parse(*body.InstanceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid instance_id")
			return
		}
		req.InstanceID = &id
	}

	incidentID, err := d.BreakGlass.Invoke(r.Context(), actor, req)
	if err != nil {
		d.Logger.Error("invoke breakglass", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"incident_id": incidentID.String()})
}

// ListIncidents returns break-glass incidents.
// GET /api/v1/internal/breakglass/incidents
func (d *Deps) ListIncidents(w http.ResponseWriter, r *http.Request) {
	incidents, err := d.BreakGlass.ListIncidents(r.Context(), d.Pool, 50, 0)
	if err != nil {
		d.Logger.Error("list incidents", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents})
}

// ReviewIncident submits a review for a break-glass incident.
// POST /api/v1/internal/breakglass/incidents/:id/review
func (d *Deps) ReviewIncident(w http.ResponseWriter, r *http.Request, incidentIDStr string) {
	incidentID, err := uuid.Parse(incidentIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid incident id")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	user, _ := middleware.UserFromCtx(r.Context())
	var reviewerID uuid.UUID
	if user != nil {
		reviewerID = user.ID
	}

	if err := d.BreakGlass.ReviewIncident(r.Context(), d.Pool, incidentID, reviewerID, body.Note); err != nil {
		d.Logger.Error("review incident", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reviewed"})
}

// -----------------------------------------------------------------------
// Tokens
// -----------------------------------------------------------------------

// ListTokens returns the current user's API tokens (without raw values).
// GET /api/v1/internal/tokens/
func (d *Deps) ListTokens(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, name, scopes, last_used, expires_at, created_at
		FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC
	`, user.ID)
	if err != nil {
		d.Logger.Error("list tokens", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type tokenRow struct {
		ID        uuid.UUID  `json:"id"`
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		LastUsed  *time.Time `json:"last_used,omitempty"`
		ExpiresAt *time.Time `json:"expires_at,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
	}
	var tokens []tokenRow
	for rows.Next() {
		var t tokenRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Scopes, &t.LastUsed, &t.ExpiresAt, &t.CreatedAt); err != nil {
			continue
		}
		tokens = append(tokens, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// CreateToken creates an API token and returns the raw token (shown once).
// POST /api/v1/internal/tokens/
func (d *Deps) CreateToken(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt *string  `json:"expires_at,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	var expiresAt *time.Time
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid expires_at (RFC3339 required)")
			return
		}
		expiresAt = &t
	}

	rawToken, tokenHash := generateAPIToken()
	var tokenID uuid.UUID
	err := d.Pool.QueryRow(r.Context(), `
		INSERT INTO api_tokens (user_id, name, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, user.ID, body.Name, tokenHash, body.Scopes, expiresAt).Scan(&tokenID)
	if err != nil {
		d.Logger.Error("create token", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":    tokenID,
		"token": rawToken,
		"name":  body.Name,
	})
}

// RevokeToken deletes one of the current user's API tokens.
// DELETE /api/v1/internal/tokens/:id
func (d *Deps) RevokeToken(w http.ResponseWriter, r *http.Request, tokenIDStr string) {
	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token id")
		return
	}
	user, ok := middleware.UserFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	_, err = d.Pool.Exec(r.Context(), `
		DELETE FROM api_tokens WHERE id = $1 AND user_id = $2
	`, tokenID, user.ID)
	if err != nil {
		d.Logger.Error("revoke token", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// -----------------------------------------------------------------------
// Admin — instances
// -----------------------------------------------------------------------

// ListInstances returns all integration instances.
// GET /api/v1/internal/admin/instances
func (d *Deps) ListInstances(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, name, plugin_id, is_active, last_health_ok, last_health_at, created_at
		FROM integration_instances ORDER BY name ASC
	`)
	if err != nil {
		d.Logger.Error("list instances", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type instRow struct {
		ID           uuid.UUID  `json:"id"`
		Name         string     `json:"name"`
		PluginID     string     `json:"plugin_id"`
		IsActive     bool       `json:"is_active"`
		LastHealthOK bool       `json:"last_health_ok"`
		LastHealthAt *time.Time `json:"last_health_at,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
	}
	var instances []instRow
	for rows.Next() {
		var inst instRow
		if err := rows.Scan(&inst.ID, &inst.Name, &inst.PluginID, &inst.IsActive, &inst.LastHealthOK, &inst.LastHealthAt, &inst.CreatedAt); err != nil {
			continue
		}
		instances = append(instances, inst)
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": instances})
}

// CreateInstance creates an integration instance.
// POST /api/v1/internal/admin/instances
func (d *Deps) CreateInstance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		PluginID string `json:"plugin_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var id uuid.UUID
	if err := d.Pool.QueryRow(r.Context(), `
		INSERT INTO integration_instances (name, plugin_id) VALUES ($1, $2) RETURNING id
	`, body.Name, body.PluginID).Scan(&id); err != nil {
		d.Logger.Error("create instance", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

// UpdateInstance updates an integration instance.
// PUT /api/v1/internal/admin/instances/:id
func (d *Deps) UpdateInstance(w http.ResponseWriter, r *http.Request, instanceIDStr string) {
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance id")
		return
	}
	var body struct {
		Name     string `json:"name"`
		IsActive *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := d.Pool.Exec(r.Context(), `
		UPDATE integration_instances
		SET name = COALESCE(NULLIF($2, ''), name), is_active = COALESCE($3, is_active)
		WHERE id = $1
	`, instanceID, body.Name, body.IsActive); err != nil {
		d.Logger.Error("update instance", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteInstance deletes an integration instance.
// DELETE /api/v1/internal/admin/instances/:id
func (d *Deps) DeleteInstance(w http.ResponseWriter, r *http.Request, instanceIDStr string) {
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid instance id")
		return
	}
	if _, err := d.Pool.Exec(r.Context(), `DELETE FROM integration_instances WHERE id = $1`, instanceID); err != nil {
		d.Logger.Error("delete instance", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -----------------------------------------------------------------------
// Admin — roles
// -----------------------------------------------------------------------

// ListRoles returns all roles with their permissions.
// GET /api/v1/internal/admin/roles
func (d *Deps) ListRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT r.id, r.name, r.description,
		       array_agg(rp.permission) FILTER (WHERE rp.permission IS NOT NULL) AS permissions
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		GROUP BY r.id, r.name, r.description ORDER BY r.name ASC
	`)
	if err != nil {
		d.Logger.Error("list roles", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type roleRow struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Permissions []string  `json:"permissions"`
	}
	var result []roleRow
	for rows.Next() {
		var row roleRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &row.Permissions); err != nil {
			continue
		}
		result = append(result, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": result})
}

// AssignRole grants a role to a user.
// POST /api/v1/internal/admin/roles/:name/assign
func (d *Deps) AssignRole(w http.ResponseWriter, r *http.Request, roleName string) {
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	roleID, err := rbac.RoleIDByName(r.Context(), d.Pool, roleName)
	if err != nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	var granterID uuid.UUID
	if u, ok := middleware.UserFromCtx(r.Context()); ok {
		granterID = u.ID
	}
	if err := d.Checker.AssignRole(r.Context(), d.Pool, userID, roleID, granterID); err != nil {
		d.Logger.Error("assign role", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// RevokeRole removes a role from a user.
// DELETE /api/v1/internal/admin/roles/:name/users/:userId
func (d *Deps) RevokeRole(w http.ResponseWriter, r *http.Request, roleName, userIDStr string) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	roleID, err := rbac.RoleIDByName(r.Context(), d.Pool, roleName)
	if err != nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if err := d.Checker.RevokeRole(r.Context(), d.Pool, userID, roleID); err != nil {
		d.Logger.Error("revoke role", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// -----------------------------------------------------------------------
// Admin — PBAC
// -----------------------------------------------------------------------

// ListPBACPolicies returns all PBAC policy rows.
// GET /api/v1/internal/admin/pbac
func (d *Deps) ListPBACPolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, name, policy_type, is_enabled, config, created_at, updated_at
		FROM pbac_policies ORDER BY name ASC
	`)
	if err != nil {
		d.Logger.Error("list pbac policies", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type policyRow struct {
		ID         uuid.UUID       `json:"id"`
		Name       string          `json:"name"`
		PolicyType string          `json:"policy_type"`
		IsEnabled  bool            `json:"is_enabled"`
		Config     json.RawMessage `json:"config"`
		CreatedAt  time.Time       `json:"created_at"`
		UpdatedAt  time.Time       `json:"updated_at"`
	}
	var result []policyRow
	for rows.Next() {
		var row policyRow
		if err := rows.Scan(&row.ID, &row.Name, &row.PolicyType, &row.IsEnabled, &row.Config, &row.CreatedAt, &row.UpdatedAt); err != nil {
			continue
		}
		result = append(result, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": result})
}

// UpdatePBACPolicy updates a PBAC policy by name.
// PUT /api/v1/internal/admin/pbac/:name
func (d *Deps) UpdatePBACPolicy(w http.ResponseWriter, r *http.Request, policyName string) {
	var body struct {
		IsEnabled *bool           `json:"is_enabled"`
		Config    json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	configArg := nilIfEmpty(body.Config)
	if _, err := d.Pool.Exec(r.Context(), `
		UPDATE pbac_policies
		SET is_enabled = COALESCE($2, is_enabled),
		    config     = COALESCE($3::jsonb, config),
		    updated_at = now()
		WHERE name = $1
	`, policyName, body.IsEnabled, configArg); err != nil {
		d.Logger.Error("update pbac policy", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -----------------------------------------------------------------------
// Admin — hold templates
// -----------------------------------------------------------------------

// ListHoldTemplates returns all hold templates.
// GET /api/v1/internal/admin/hold-templates
func (d *Deps) ListHoldTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := d.HoldSvc.ListTemplates(r.Context(), d.Pool)
	if err != nil {
		d.Logger.Error("list hold templates", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

// CreateHoldTemplate creates a hold template.
// POST /api/v1/internal/admin/hold-templates
func (d *Deps) CreateHoldTemplate(w http.ResponseWriter, r *http.Request) {
	var body legalhold.CreateTemplateParams
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	tpl, err := d.HoldSvc.CreateTemplate(r.Context(), tx, actor, body)
	if err != nil {
		d.Logger.Error("create hold template", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusCreated, map[string]any{"template": tpl})
}

// UpdateHoldTemplate updates a hold template.
// PUT /api/v1/internal/admin/hold-templates/:id
func (d *Deps) UpdateHoldTemplate(w http.ResponseWriter, r *http.Request, templateIDStr string) {
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	var body legalhold.UpdateTemplateParams
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	tpl, err := d.HoldSvc.UpdateTemplate(r.Context(), tx, actor, templateID, body)
	if err != nil {
		d.Logger.Error("update hold template", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"template": tpl})
}

// DeleteHoldTemplate deletes a hold template.
// DELETE /api/v1/internal/admin/hold-templates/:id
func (d *Deps) DeleteHoldTemplate(w http.ResponseWriter, r *http.Request, templateIDStr string) {
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id")
		return
	}
	user, _ := middleware.UserFromCtx(r.Context())
	actor := actorFromUser(user)

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if err := d.HoldSvc.DeleteTemplate(r.Context(), tx, actor, templateID); err != nil {
		d.Logger.Error("delete hold template", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -----------------------------------------------------------------------
// Admin — VIP identities
// -----------------------------------------------------------------------

// ListVIP returns all VIP identity entries.
// GET /api/v1/internal/admin/vip
func (d *Deps) ListVIP(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, email, reason, added_by, created_at FROM vip_identities ORDER BY email ASC
	`)
	if err != nil {
		d.Logger.Error("list vip", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type vipRow struct {
		ID        uuid.UUID  `json:"id"`
		Email     string     `json:"email"`
		Reason    string     `json:"reason"`
		AddedBy   *uuid.UUID `json:"added_by,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
	}
	var vips []vipRow
	for rows.Next() {
		var v vipRow
		if err := rows.Scan(&v.ID, &v.Email, &v.Reason, &v.AddedBy, &v.CreatedAt); err != nil {
			continue
		}
		vips = append(vips, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"vip_identities": vips})
}

// AddVIP adds an email to the VIP list.
// POST /api/v1/internal/admin/vip
func (d *Deps) AddVIP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email  string `json:"email"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var addedBy *uuid.UUID
	if u, ok := middleware.UserFromCtx(r.Context()); ok {
		addedBy = &u.ID
	}
	var id uuid.UUID
	if err := d.Pool.QueryRow(r.Context(), `
		INSERT INTO vip_identities (email, reason, added_by) VALUES ($1, $2, $3) RETURNING id
	`, strings.ToLower(body.Email), body.Reason, addedBy).Scan(&id); err != nil {
		d.Logger.Error("add vip", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id.String()})
}

// RemoveVIP removes a VIP identity.
// DELETE /api/v1/internal/admin/vip/:id
func (d *Deps) RemoveVIP(w http.ResponseWriter, r *http.Request, vipIDStr string) {
	vipID, err := uuid.Parse(vipIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid vip id")
		return
	}
	if _, err := d.Pool.Exec(r.Context(), `DELETE FROM vip_identities WHERE id = $1`, vipID); err != nil {
		d.Logger.Error("remove vip", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// -----------------------------------------------------------------------
// Assistant (stub)
// -----------------------------------------------------------------------

// AssistantStream is a stub SSE endpoint; actual Claude integration is deferred.
// GET /api/v1/internal/assistant/stream
func (d *Deps) AssistantStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"ready\"}\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func actorFromUser(u *domain.User) domain.Actor {
	if u == nil {
		return domain.Actor{Type: domain.ActorTypeUser}
	}
	return domain.Actor{ID: u.ID, Type: domain.ActorTypeUser, Email: u.Email}
}

type approvalRow struct {
	ID          uuid.UUID       `json:"id"`
	RequesterID uuid.UUID       `json:"requester_id"`
	ActionKey   string          `json:"action_key"`
	InstanceID  *uuid.UUID      `json:"instance_id,omitempty"`
	TargetEmail *string         `json:"target_email,omitempty"`
	Params      json.RawMessage `json:"params"`
	Reason      *string         `json:"reason,omitempty"`
	Status      string          `json:"status"`
	ReviewerID  *uuid.UUID      `json:"reviewer_id,omitempty"`
	ReviewNote  *string         `json:"review_note,omitempty"`
	ExpiresAt   time.Time       `json:"expires_at"`
	ReviewedAt  *time.Time      `json:"reviewed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

func scanApprovals(rows pgx.Rows) []approvalRow {
	var result []approvalRow
	for rows.Next() {
		var row approvalRow
		if err := rows.Scan(
			&row.ID, &row.RequesterID, &row.ActionKey, &row.InstanceID,
			&row.TargetEmail, &row.Params, &row.Reason, &row.Status,
			&row.ReviewerID, &row.ReviewNote, &row.ExpiresAt, &row.ReviewedAt,
			&row.CreatedAt,
		); err != nil {
			continue
		}
		result = append(result, row)
	}
	return result
}

func scanEvents(rows pgx.Rows) []domain.Event {
	var out []domain.Event
	for rows.Next() {
		e, ok := scanOneEvent(rows)
		if ok {
			out = append(out, e)
		}
	}
	return out
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOneEvent(r rowScanner) (domain.Event, bool) {
	var e domain.Event
	var aggTypeStr string
	var payloadBytes []byte
	var actorID *uuid.UUID
	var actorType *string
	if err := r.Scan(&e.ID, &aggTypeStr, &e.AggregateID, &e.Version, &e.Type, &payloadBytes, &actorID, &actorType, &e.CreatedAt); err != nil {
		return domain.Event{}, false
	}
	e.AggregateType = domain.AggregateType(aggTypeStr)
	if actorID != nil {
		e.ActorID = actorID
	}
	if actorType != nil {
		e.ActorType = domain.ActorType(*actorType)
	}
	if len(payloadBytes) > 0 {
		_ = json.Unmarshal(payloadBytes, &e.Payload)
	}
	return e, true
}

func nilIfEmpty(b json.RawMessage) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}
