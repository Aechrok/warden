// Package internal implements Warden's session-authenticated internal API.
package internal

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/aechrok/warden/internal/crypto"
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

// upstreamHTTPStatuser is satisfied by httpx.APIError without requiring an
// import of the plugins/internal/httpx package (which is internal to plugins/).
type upstreamHTTPStatuser interface {
	UpstreamHTTPStatus() int
}

// upstreamStatus returns the HTTP status to forward to the client for an
// upstream plugin error. 401/403/404 from the upstream map 1:1; everything
// else becomes 502.
func upstreamStatus(err error) int {
	var ae upstreamHTTPStatuser
	if errors.As(err, &ae) {
		switch ae.UpstreamHTTPStatus() {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return ae.UpstreamHTTPStatus()
		}
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// upstreamMessage returns a human-readable error message from an upstream
// plugin error, falling back to a generic message for internal errors.
func upstreamMessage(err error) string {
	var ae upstreamHTTPStatuser
	if errors.As(err, &ae) {
		switch ae.UpstreamHTTPStatus() {
		case http.StatusUnauthorized:
			return "upstream rejected request: 401 Unauthorized — check integration credentials"
		case http.StatusForbidden:
			return "upstream rejected request: 403 Forbidden — token lacks required permissions"
		case http.StatusNotFound:
			return "identity not found in this integration"
		}
		return fmt.Sprintf("upstream error: %d", ae.UpstreamHTTPStatus())
	}
	return err.Error()
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
	EncKey     []byte
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

// SearchIdentities resolves an email against a specific integration instance,
// or returns all cached results across instances when instance_id is omitted.
// GET /api/v1/internal/identities/search?email=&instance_id=
func (d *Deps) SearchIdentities(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}

	var onHold bool
	_ = d.Pool.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM legal_hold_custodians c
			JOIN legal_holds lh ON lh.id = c.hold_id
			WHERE lower(c.email) = lower($1)
			  AND c.removed_at IS NULL
			  AND lh.status = 'active'
		)
	`, email).Scan(&onHold)

	instanceIDStr := strings.TrimSpace(r.URL.Query().Get("instance_id"))
	if instanceIDStr != "" {
		// Live lookup for a specific instance.
		instanceID, err := uuid.Parse(instanceIDStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid instance_id")
			return
		}
		identity, err := d.Dispatcher.GetIdentity(r.Context(), instanceID, email)
		if err != nil {
			d.Logger.Error("search identities", zap.Error(err))
			writeError(w, upstreamStatus(err), upstreamMessage(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"identities": []domain.Identity{identity},
			"on_hold":    onHold,
		})
		return
	}

	// No instance specified — fan out live lookups across all active instances.
	instRows, err := d.Pool.Query(r.Context(), `
		SELECT id FROM integration_instances WHERE is_active = true ORDER BY name ASC
	`)
	if err != nil {
		d.Logger.Error("search identities: list instances", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var instanceIDs []uuid.UUID
	for instRows.Next() {
		var iid uuid.UUID
		if err := instRows.Scan(&iid); err == nil {
			instanceIDs = append(instanceIDs, iid)
		}
	}
	instRows.Close()

	identities := []domain.Identity{}
	for _, iid := range instanceIDs {
		id, err := d.Dispatcher.GetIdentity(r.Context(), iid, email)
		if err != nil {
			d.Logger.Warn("search identities: skip instance", zap.Stringer("instance_id", iid), zap.Error(err))
			continue
		}
		identities = append(identities, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"identities": identities,
		"on_hold":    onHold,
	})
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

// ListActions returns all available actions as a flat list, each decorated
// with the instance_id and plugin they belong to.
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

	type actionItem struct {
		Key              string   `json:"key"`
		Label            string   `json:"label"`
		Description      string   `json:"description"`
		InstanceID       string   `json:"instance_id"`
		Plugin           string   `json:"plugin"`
		Destructive      bool     `json:"destructive"`
		RequiresApproval bool     `json:"requires_approval"`
		ApplicableStates []string `json:"applicable_states,omitempty"`
	}
	var result []actionItem
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
		for _, a := range exec.Actions() {
			result = append(result, actionItem{
				Key:              a.Key,
				Label:            a.Label,
				Description:      a.Description,
				InstanceID:       id.String(),
				Plugin:           pluginID,
				Destructive:      a.Destructive,
				RequiresApproval: a.RequiresApproval,
				ApplicableStates: a.ApplicableStates,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"actions": result})
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
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		TemplateID      *string  `json:"template_id,omitempty"`
		ExpiresAt       *string  `json:"expires_at,omitempty"`
		CustodianEmails []string `json:"custodian_emails,omitempty"`
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

	var failedCustodians []string
	for _, email := range body.CustodianEmails {
		email = strings.TrimSpace(strings.ToLower(email))
		if email == "" {
			continue
		}
		if err := d.HoldSvc.AddCustodian(r.Context(), tx, hold.ID, email, actor); err != nil {
			d.Logger.Warn("create hold: add custodian", zap.String("email", email), zap.Error(err))
			failedCustodians = append(failedCustodians, email)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp := map[string]any{"hold": hold}
	if len(failedCustodians) > 0 {
		resp["failed_custodians"] = failedCustodians
		resp["warning"] = "some custodians could not be added"
	}
	writeJSON(w, http.StatusCreated, resp)
}

// ListHolds lists holds enriched with custodians, cascade states, and placed-by name.
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

	type holdEnriched struct {
		*domain.Hold
		PlacedByName  string                `json:"placed_by_name,omitempty"`
		Custodians    []domain.Custodian    `json:"custodians"`
		CascadeStates []domain.CascadeState `json:"cascade_states"`
	}

	if len(holds) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"holds": []holdEnriched{}})
		return
	}

	holdIDStrs := make([]string, len(holds))
	var placedByStrs []string
	seenUsers := map[uuid.UUID]bool{}
	for i, h := range holds {
		holdIDStrs[i] = h.ID.String()
		if h.PlacedBy != nil && !seenUsers[*h.PlacedBy] {
			placedByStrs = append(placedByStrs, h.PlacedBy.String())
			seenUsers[*h.PlacedBy] = true
		}
	}

	custByHold := map[uuid.UUID][]domain.Custodian{}
	custRows, e := d.Pool.Query(r.Context(), `
		SELECT id, hold_id, email, added_by, created_at
		FROM legal_hold_custodians
		WHERE hold_id = ANY($1::uuid[]) AND removed_at IS NULL
		ORDER BY created_at ASC
	`, holdIDStrs)
	if e != nil {
		d.Logger.Error("list holds: query custodians", zap.Error(e))
	} else {
		for custRows.Next() {
			var c domain.Custodian
			if err := custRows.Scan(&c.ID, &c.HoldID, &c.Email, &c.AddedBy, &c.CreatedAt); err != nil {
				d.Logger.Warn("list holds: scan custodian", zap.Error(err))
				continue
			}
			custByHold[c.HoldID] = append(custByHold[c.HoldID], c)
		}
		custRows.Close()
		if err := custRows.Err(); err != nil {
			d.Logger.Error("list holds: custodians rows", zap.Error(err))
		}
	}

	cssByHold := map[uuid.UUID][]domain.CascadeState{}
	csRows, e := d.Pool.Query(r.Context(), `
		SELECT id, hold_id, custodian_email, instance_id, status, COALESCE(last_error, ''), attempts, completed_at, created_at, updated_at
		FROM cascade_state WHERE hold_id = ANY($1::uuid[])
	`, holdIDStrs)
	if e != nil {
		d.Logger.Error("list holds: query cascade states", zap.Error(e))
	} else {
		for csRows.Next() {
			var cs domain.CascadeState
			if err := csRows.Scan(&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID, &cs.Status, &cs.LastError, &cs.Attempts, &cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt); err != nil {
				d.Logger.Warn("list holds: scan cascade state", zap.Error(err))
				continue
			}
			cssByHold[cs.HoldID] = append(cssByHold[cs.HoldID], cs)
		}
		csRows.Close()
		if err := csRows.Err(); err != nil {
			d.Logger.Error("list holds: cascade state rows", zap.Error(err))
		}
	}

	userNames := map[string]string{}
	if len(placedByStrs) > 0 {
		uRows, e := d.Pool.Query(r.Context(), `
			SELECT id, COALESCE(NULLIF(name, ''), email) FROM users WHERE id = ANY($1::uuid[])
		`, placedByStrs)
		if e != nil {
			d.Logger.Error("list holds: query user names", zap.Error(e))
		} else {
			for uRows.Next() {
				var id uuid.UUID
				var name string
				if err := uRows.Scan(&id, &name); err != nil {
					d.Logger.Warn("list holds: scan user name", zap.Error(err))
					continue
				}
				userNames[id.String()] = name
			}
			uRows.Close()
			if err := uRows.Err(); err != nil {
				d.Logger.Error("list holds: user names rows", zap.Error(err))
			}
		}
	}

	out := make([]holdEnriched, len(holds))
	for i, h := range holds {
		name := ""
		if h.PlacedBy != nil {
			name = userNames[h.PlacedBy.String()]
		}
		custodians := custByHold[h.ID]
		if custodians == nil {
			custodians = []domain.Custodian{}
		}
		css := cssByHold[h.ID]
		if css == nil {
			css = []domain.CascadeState{}
		}
		out[i] = holdEnriched{
			Hold:          h,
			PlacedByName:  name,
			Custodians:    custodians,
			CascadeStates: css,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"holds": out})
}

// GetHold returns a hold with its custodians and cascade states.
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

	placedByName := ""
	if hold.PlacedBy != nil {
		_ = d.Pool.QueryRow(r.Context(), `
			SELECT COALESCE(NULLIF(name, ''), email) FROM users WHERE id = $1
		`, *hold.PlacedBy).Scan(&placedByName)
	}

	custodians := []domain.Custodian{}
	custRows, e := d.Pool.Query(r.Context(), `
		SELECT id, hold_id, email, added_by, created_at
		FROM legal_hold_custodians
		WHERE hold_id = $1 AND removed_at IS NULL
		ORDER BY created_at ASC
	`, holdID)
	if e != nil {
		d.Logger.Error("get hold: query custodians", zap.Error(e))
	} else {
		for custRows.Next() {
			var c domain.Custodian
			if err := custRows.Scan(&c.ID, &c.HoldID, &c.Email, &c.AddedBy, &c.CreatedAt); err != nil {
				d.Logger.Warn("get hold: scan custodian", zap.Error(err))
				continue
			}
			custodians = append(custodians, c)
		}
		custRows.Close()
		if err := custRows.Err(); err != nil {
			d.Logger.Error("get hold: custodians rows", zap.Error(err))
		}
	}

	cascadeStates := []domain.CascadeState{}
	csRows, e := d.Pool.Query(r.Context(), `
		SELECT id, hold_id, custodian_email, instance_id, status, COALESCE(last_error, ''), attempts, completed_at, created_at, updated_at
		FROM cascade_state WHERE hold_id = $1 ORDER BY created_at ASC
	`, holdID)
	if e != nil {
		d.Logger.Error("get hold: query cascade states", zap.Error(e))
	} else {
		for csRows.Next() {
			var cs domain.CascadeState
			if err := csRows.Scan(&cs.ID, &cs.HoldID, &cs.CustodianEmail, &cs.InstanceID, &cs.Status, &cs.LastError, &cs.Attempts, &cs.CompletedAt, &cs.CreatedAt, &cs.UpdatedAt); err != nil {
				d.Logger.Warn("get hold: scan cascade state", zap.Error(err))
				continue
			}
			cascadeStates = append(cascadeStates, cs)
		}
		csRows.Close()
		if err := csRows.Err(); err != nil {
			d.Logger.Error("get hold: cascade state rows", zap.Error(err))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hold":           hold,
		"placed_by_name": placedByName,
		"custodians":     custodians,
		"cascade_states": cascadeStates,
	})
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
	writeJSON(w, http.StatusOK, map[string]any{"events": enrichEventsWithActorNames(r.Context(), d.Pool, events)})
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

	tag, err := d.Pool.Exec(r.Context(), `
		UPDATE approval_requests
		SET status = $2, reviewer_id = $3, review_note = $4, reviewed_at = now()
		WHERE id = $1 AND status = 'pending' AND requester_id != $3
	`, approvalID, newStatus, reviewerID, body.Note)
	if err != nil {
		d.Logger.Error("decide approval", zap.String("status", newStatus), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "approval not found, already decided, or self-approval not permitted")
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

// ListPlugins returns all registered plugins with their credential schemas.
// GET /api/v1/internal/admin/plugins
func (d *Deps) ListPlugins(w http.ResponseWriter, r *http.Request) {
	type pluginInfo struct {
		ID      string                   `json:"id"`
		Name    string                   `json:"name"`
		Schema  []domain.CredentialField `json:"schema"`
		Loaded  bool                     `json:"loaded"`
	}

	// Determine which plugin IDs have at least one active instance.
	rows, err := d.Pool.Query(r.Context(), `
		SELECT DISTINCT plugin_id FROM integration_instances WHERE is_active = true
	`)
	if err != nil {
		d.Logger.Error("list plugins: query instances", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()
	activeIDs := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			activeIDs[id] = true
		}
	}

	loadedOnly := r.URL.Query().Get("loaded_only") == "true"
	all := plugin.All()
	result := make([]pluginInfo, 0, len(all))
	for _, p := range all {
		loaded := activeIDs[p.ID()]
		if loadedOnly && !loaded {
			continue
		}
		result = append(result, pluginInfo{
			ID:     p.ID(),
			Name:   p.Name(),
			Schema: p.CredentialSchema(),
			Loaded: loaded,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": result})
}

// ListInstances returns all integration instances.
// GET /api/v1/internal/admin/instances
func (d *Deps) ListInstances(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, name, plugin_id, is_active, COALESCE(last_health_ok, false), last_health_at, created_at
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
		Name        string            `json:"name"`
		PluginID    string            `json:"plugin_id"`
		Credentials map[string]string `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var credsEnc []byte
	if len(body.Credentials) > 0 && len(d.EncKey) == 32 {
		credsJSON, err := json.Marshal(body.Credentials)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid credentials")
			return
		}
		enc, err := crypto.Encrypt(d.EncKey, credsJSON)
		if err != nil {
			d.Logger.Error("create instance: encrypt credentials", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		credsEnc = enc
	}

	var id uuid.UUID
	if err := d.Pool.QueryRow(r.Context(), `
		INSERT INTO integration_instances (name, plugin_id, credentials_enc) VALUES ($1, $2, $3) RETURNING id
	`, body.Name, body.PluginID, credsEnc).Scan(&id); err != nil {
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
		Name        string            `json:"name"`
		IsActive    *bool             `json:"is_active"`
		Credentials map[string]string `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(body.Credentials) > 0 && len(d.EncKey) == 32 {
		// Merge into existing credentials so fields not included in the request
		// (e.g. a secret the UI blanks out) are not wiped.
		var existingEnc []byte
		_ = d.Pool.QueryRow(r.Context(), `SELECT credentials_enc FROM integration_instances WHERE id = $1`, instanceID).Scan(&existingEnc)
		merged := map[string]string{}
		if len(existingEnc) > 0 {
			if pt, err := crypto.Decrypt(d.EncKey, existingEnc); err == nil {
				_ = json.Unmarshal(pt, &merged)
			}
		}
		for k, v := range body.Credentials {
			if v != "" {
				merged[k] = v
			}
		}
		credsJSON, err := json.Marshal(merged)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid credentials")
			return
		}
		enc, err := crypto.Encrypt(d.EncKey, credsJSON)
		if err != nil {
			d.Logger.Error("update instance: encrypt credentials", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if _, err := d.Pool.Exec(r.Context(), `
			UPDATE integration_instances
			SET name = COALESCE(NULLIF($2, ''), name),
			    is_active = COALESCE($3, is_active),
			    credentials_enc = $4
			WHERE id = $1
		`, instanceID, body.Name, body.IsActive, enc); err != nil {
			d.Logger.Error("update instance", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	} else {
		if _, err := d.Pool.Exec(r.Context(), `
			UPDATE integration_instances
			SET name = COALESCE(NULLIF($2, ''), name), is_active = COALESCE($3, is_active)
			WHERE id = $1
		`, instanceID, body.Name, body.IsActive); err != nil {
			d.Logger.Error("update instance", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
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

// ListRoles returns all roles with their permissions and builtin flag.
// GET /api/v1/internal/admin/roles
func (d *Deps) ListRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT r.id, r.name, r.description, r.is_builtin,
		       array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL) AS permissions
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		GROUP BY r.id, r.name, r.description, r.is_builtin ORDER BY r.name ASC
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
		IsBuiltin   bool      `json:"is_builtin"`
		Permissions []string  `json:"permissions"`
	}
	var result []roleRow
	for rows.Next() {
		var row roleRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Description, &row.IsBuiltin, &row.Permissions); err != nil {
			continue
		}
		if row.Permissions == nil {
			row.Permissions = []string{}
		}
		result = append(result, row)
	}
	if result == nil {
		result = []roleRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": result})
}

// ListUsers returns all users with their assigned roles and origin.
// GET /api/v1/internal/admin/users
func (d *Deps) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT u.id, u.email, u.name, u.is_active, u.origin, u.created_at,
		       array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL) AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id, u.email, u.name, u.is_active, u.origin, u.created_at
		ORDER BY u.email ASC
	`)
	if err != nil {
		d.Logger.Error("list users", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type userRow struct {
		ID        uuid.UUID `json:"id"`
		Email     string    `json:"email"`
		Name      string    `json:"name"`
		IsActive  bool      `json:"is_active"`
		Origin    string    `json:"origin"`
		CreatedAt time.Time `json:"created_at"`
		Roles     []string  `json:"roles"`
	}
	var result []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.IsActive, &u.Origin, &u.CreatedAt, &u.Roles); err != nil {
			continue
		}
		if u.Roles == nil {
			u.Roles = []string{}
		}
		result = append(result, u)
	}
	if result == nil {
		result = []userRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": result})
}

// ListPermissions returns all canonical permissions.
// GET /api/v1/internal/admin/permissions
func (d *Deps) ListPermissions(w http.ResponseWriter, r *http.Request) {
	all := rbac.AllPermissions()
	result := make([]string, len(all))
	for i, p := range all {
		result[i] = p.String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": result})
}

// CreateRole creates a custom (non-builtin) role with optional permissions.
// POST /api/v1/internal/admin/roles
func (d *Deps) CreateRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	for _, perm := range body.Permissions {
		if !rbac.IsKnown(rbac.Permission(perm)) {
			writeError(w, http.StatusBadRequest, "unknown permission: "+perm)
			return
		}
	}

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var roleID uuid.UUID
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO roles (name, description, is_builtin) VALUES ($1, $2, false) RETURNING id
	`, body.Name, body.Description).Scan(&roleID); err != nil {
		d.Logger.Error("create role", zap.Error(err))
		writeError(w, http.StatusConflict, "role name already exists")
		return
	}
	for _, perm := range body.Permissions {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING
		`, roleID, perm); err != nil {
			d.Logger.Error("create role: insert permission", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": roleID.String()})
}

// UpdateRolePermissions replaces the permission set on a custom (non-builtin) role.
// PUT /api/v1/internal/admin/roles/:name/permissions
func (d *Deps) UpdateRolePermissions(w http.ResponseWriter, r *http.Request, roleName string) {
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, perm := range body.Permissions {
		if !rbac.IsKnown(rbac.Permission(perm)) {
			writeError(w, http.StatusBadRequest, "unknown permission: "+perm)
			return
		}
	}

	tx, err := d.Pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	var roleID uuid.UUID
	var isBuiltin bool
	if err := tx.QueryRow(r.Context(), `SELECT id, is_builtin FROM roles WHERE name = $1`, roleName).Scan(&roleID, &isBuiltin); err != nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if isBuiltin {
		writeError(w, http.StatusForbidden, "cannot modify built-in role permissions")
		return
	}

	if _, err := tx.Exec(r.Context(), `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	for _, perm := range body.Permissions {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2) ON CONFLICT DO NOTHING
		`, roleID, perm); err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteRole deletes a custom (non-builtin) role.
// DELETE /api/v1/internal/admin/roles/:name
func (d *Deps) DeleteRole(w http.ResponseWriter, r *http.Request, roleName string) {
	var isBuiltin bool
	if err := d.Pool.QueryRow(r.Context(), `SELECT is_builtin FROM roles WHERE name = $1`, roleName).Scan(&isBuiltin); err != nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if isBuiltin {
		writeError(w, http.StatusForbidden, "cannot delete built-in role")
		return
	}
	if _, err := d.Pool.Exec(r.Context(), `DELETE FROM roles WHERE name = $1 AND is_builtin = false`, roleName); err != nil {
		d.Logger.Error("delete role", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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
	var body struct {
		Name           string `json:"name"`
		Description    string `json:"description"`
		ProviderGlob   string `json:"provider_glob"`
		ExpirationDays *int   `json:"expiration_days"`
		IsDefault      bool   `json:"is_default"`
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

	tpl, err := d.HoldSvc.CreateTemplate(r.Context(), tx, actor, legalhold.CreateTemplateParams{
		Name:           body.Name,
		Description:    body.Description,
		ProviderGlob:   body.ProviderGlob,
		ExpirationDays: body.ExpirationDays,
		IsDefault:      body.IsDefault,
	})
	if err != nil {
		d.Logger.Error("create hold template", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
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
	var body struct {
		Name           string `json:"name"`
		Description    string `json:"description"`
		ProviderGlob   string `json:"provider_glob"`
		ExpirationDays *int   `json:"expiration_days"`
		IsDefault      bool   `json:"is_default"`
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

	tpl, err := d.HoldSvc.UpdateTemplate(r.Context(), tx, actor, templateID, legalhold.UpdateTemplateParams{
		Name:           body.Name,
		Description:    body.Description,
		ProviderGlob:   body.ProviderGlob,
		ExpirationDays: body.ExpirationDays,
		IsDefault:      body.IsDefault,
	})
	if err != nil {
		d.Logger.Error("update hold template", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
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
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -----------------------------------------------------------------------
// Admin — SCIM groups
// -----------------------------------------------------------------------

// ListSCIMGroups returns all SCIM groups with their bound role, if any.
// GET /api/v1/internal/admin/scim-groups
func (d *Deps) ListSCIMGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT g.id, g.external_id, g.name, g.role_id, r.name, g.created_at, g.updated_at
		FROM scim_groups g
		LEFT JOIN roles r ON r.id = g.role_id
		ORDER BY g.name ASC
	`)
	if err != nil {
		d.Logger.Error("list scim groups", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type groupRow struct {
		ID         uuid.UUID  `json:"id"`
		ExternalID string     `json:"external_id"`
		Name       string     `json:"name"`
		RoleID     *uuid.UUID `json:"role_id,omitempty"`
		RoleName   *string    `json:"role_name,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
		UpdatedAt  time.Time  `json:"updated_at"`
	}
	var result []groupRow
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.ID, &g.ExternalID, &g.Name, &g.RoleID, &g.RoleName, &g.CreatedAt, &g.UpdatedAt); err != nil {
			continue
		}
		result = append(result, g)
	}
	if result == nil {
		result = []groupRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": result})
}

// UpdateSCIMGroupRole sets or clears the role binding for a SCIM group.
// PUT /api/v1/internal/admin/scim-groups/:id/role
func (d *Deps) UpdateSCIMGroupRole(w http.ResponseWriter, r *http.Request, groupIDStr string) {
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var body struct {
		RoleID *string `json:"role_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var roleID *uuid.UUID
	if body.RoleID != nil && *body.RoleID != "" {
		rid, err := uuid.Parse(*body.RoleID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid role_id")
			return
		}
		roleID = &rid
	}
	if _, err := d.Pool.Exec(r.Context(), `
		UPDATE scim_groups SET role_id = $2, updated_at = now() WHERE id = $1
	`, groupID, roleID); err != nil {
		d.Logger.Error("update scim group role", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -----------------------------------------------------------------------
// Admin — SSO config
// -----------------------------------------------------------------------

// GetSSOConfig returns the SSO configuration. The client secret is never
// returned; only a has_secret boolean is included.
// GET /api/v1/internal/admin/sso-config
func (d *Deps) GetSSOConfig(w http.ResponseWriter, r *http.Request) {
	var (
		issuer         string
		internalIssuer string
		clientID       string
		credsEnc       []byte
		redirectURL    string
		ssoEnabled     bool
		enforceSSO     bool
		updatedAt      time.Time
	)
	err := d.Pool.QueryRow(r.Context(), `
		SELECT oidc_issuer, oidc_internal_issuer, oidc_client_id,
		       oidc_credentials_enc, oidc_redirect_url, sso_enabled, enforce_sso, updated_at
		FROM sso_config WHERE singleton = true
	`).Scan(&issuer, &internalIssuer, &clientID, &credsEnc, &redirectURL, &ssoEnabled, &enforceSSO, &updatedAt)
	if err != nil {
		d.Logger.Error("get sso config", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"oidc_issuer":          issuer,
		"oidc_internal_issuer": internalIssuer,
		"oidc_client_id":       clientID,
		"has_secret":           len(credsEnc) > 0,
		"oidc_redirect_url":    redirectURL,
		"sso_enabled":          ssoEnabled,
		"enforce_sso":          enforceSSO,
		"updated_at":           updatedAt,
	})
}

// UpdateSSOConfig saves SSO configuration. The client secret is only updated
// when a non-empty value is provided; omitting it leaves the stored secret untouched.
// PUT /api/v1/internal/admin/sso-config
func (d *Deps) UpdateSSOConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OIDCIssuer         string `json:"oidc_issuer"`
		OIDCInternalIssuer string `json:"oidc_internal_issuer"`
		OIDCClientID       string `json:"oidc_client_id"`
		OIDCClientSecret   string `json:"oidc_client_secret"`
		OIDCRedirectURL    string `json:"oidc_redirect_url"`
		SSOEnabled         bool   `json:"sso_enabled"`
		EnforceSSO         bool   `json:"enforce_sso"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.OIDCClientSecret != "" {
		var credsEnc []byte
		if len(d.EncKey) == 32 {
			enc, err := crypto.Encrypt(d.EncKey, []byte(body.OIDCClientSecret))
			if err != nil {
				d.Logger.Error("update sso config: encrypt secret", zap.Error(err))
				writeError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			credsEnc = enc
		}
		if _, err := d.Pool.Exec(r.Context(), `
			UPDATE sso_config
			SET oidc_issuer = $1, oidc_internal_issuer = $2, oidc_client_id = $3,
			    oidc_credentials_enc = $4, oidc_redirect_url = $5,
			    sso_enabled = $6, enforce_sso = $7, updated_at = now()
			WHERE singleton = true
		`, body.OIDCIssuer, body.OIDCInternalIssuer, body.OIDCClientID, credsEnc, body.OIDCRedirectURL,
			body.SSOEnabled, body.EnforceSSO); err != nil {
			d.Logger.Error("update sso config", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	} else {
		if _, err := d.Pool.Exec(r.Context(), `
			UPDATE sso_config
			SET oidc_issuer = $1, oidc_internal_issuer = $2, oidc_client_id = $3,
			    oidc_redirect_url = $4, sso_enabled = $5, enforce_sso = $6, updated_at = now()
			WHERE singleton = true
		`, body.OIDCIssuer, body.OIDCInternalIssuer, body.OIDCClientID, body.OIDCRedirectURL,
			body.SSOEnabled, body.EnforceSSO); err != nil {
			d.Logger.Error("update sso config", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// SetUserPassword bcrypt-hashes and stores a password for a user (admin only).
// PUT /api/v1/internal/admin/users/:id/password
func (d *Deps) SetUserPassword(w http.ResponseWriter, r *http.Request, userIDStr string) {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password required")
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tag, err := d.Pool.Exec(r.Context(), `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -----------------------------------------------------------------------
// Admin — Magic link invitations
// -----------------------------------------------------------------------

// ListInvitations returns all magic links.
// GET /api/v1/internal/admin/invitations
func (d *Deps) ListInvitations(w http.ResponseWriter, r *http.Request) {
	rows, err := d.Pool.Query(r.Context(), `
		SELECT m.id, m.token, m.email, m.role_name, m.label, m.used_at, m.expires_at, m.created_at,
		       u.email AS invited_by_email
		FROM magic_links m
		LEFT JOIN users u ON u.id = m.invited_by
		ORDER BY m.created_at DESC
	`)
	if err != nil {
		d.Logger.Error("list invitations", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	type invRow struct {
		ID             uuid.UUID  `json:"id"`
		Token          string     `json:"token"`
		Email          string     `json:"email"`
		RoleName       *string    `json:"role_name,omitempty"`
		Label          string     `json:"label"`
		UsedAt         *time.Time `json:"used_at,omitempty"`
		ExpiresAt      time.Time  `json:"expires_at"`
		CreatedAt      time.Time  `json:"created_at"`
		InvitedByEmail *string    `json:"invited_by_email,omitempty"`
	}
	var result []invRow
	for rows.Next() {
		var row invRow
		if err := rows.Scan(&row.ID, &row.Token, &row.Email, &row.RoleName, &row.Label,
			&row.UsedAt, &row.ExpiresAt, &row.CreatedAt, &row.InvitedByEmail); err != nil {
			continue
		}
		result = append(result, row)
	}
	if result == nil {
		result = []invRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": result})
}

// CreateInvitation generates a magic link invitation for an email address.
// POST /api/v1/internal/admin/invitations
func (d *Deps) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		RoleName string `json:"role_name"`
		Label    string `json:"label"`
		ExpiryH  int    `json:"expiry_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}
	if body.ExpiryH <= 0 {
		body.ExpiryH = 7 * 24
	}

	var invitedBy *uuid.UUID
	if u, ok := middleware.UserFromCtx(r.Context()); ok {
		invitedBy = &u.ID
	}

	var roleNameArg *string
	if body.RoleName != "" {
		roleNameArg = &body.RoleName
	}

	var id uuid.UUID
	var token string
	err := d.Pool.QueryRow(r.Context(), `
		INSERT INTO magic_links (email, role_name, label, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, now() + ($5::int * interval '1 hour'))
		RETURNING id, token
	`, email, roleNameArg, body.Label, invitedBy, body.ExpiryH).Scan(&id, &token)
	if err != nil {
		d.Logger.Error("create invitation", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id.String(), "token": token})
}

// DeleteInvitation deletes a magic link by ID.
// DELETE /api/v1/internal/admin/invitations/:id
func (d *Deps) DeleteInvitation(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := d.Pool.Exec(r.Context(), `DELETE FROM magic_links WHERE id = $1`, id); err != nil {
		d.Logger.Error("delete invitation", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
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

type enrichedEvent struct {
	domain.Event
	ActorDisplay string `json:"actor_display,omitempty"`
}

func enrichEventsWithActorNames(ctx context.Context, pool *pgxpool.Pool, events []domain.Event) []enrichedEvent {
	out := make([]enrichedEvent, len(events))
	for i, e := range events {
		out[i] = enrichedEvent{Event: e}
	}

	// Collect unique actor UUIDs.
	seen := map[uuid.UUID]bool{}
	var ids []string
	for _, e := range events {
		if e.ActorID != nil && !seen[*e.ActorID] {
			seen[*e.ActorID] = true
			ids = append(ids, e.ActorID.String())
		}
	}
	if len(ids) == 0 {
		return out
	}

	names := map[uuid.UUID]string{}
	rows, err := pool.Query(ctx, `
		SELECT id, COALESCE(NULLIF(name, ''), email) FROM users WHERE id = ANY($1::uuid[])
	`, ids)
	if err == nil {
		for rows.Next() {
			var id uuid.UUID
			var name string
			if rows.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		rows.Close()
	}

	for i, e := range events {
		if e.ActorID != nil {
			out[i].ActorDisplay = names[*e.ActorID]
		}
	}
	return out
}

func nilIfEmpty(b json.RawMessage) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}
