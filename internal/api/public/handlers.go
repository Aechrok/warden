// Package public implements the bearer-token-authenticated public API.
// Scope is checked against the token's scopes column before each operation.
package public

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/api/middleware"
	"github.com/aechrok/warden/internal/domain"
	"github.com/aechrok/warden/internal/legalhold"
	"github.com/aechrok/warden/internal/plugin"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// requireScope rejects the request with 403 if the token lacks the given scope.
func requireScope(w http.ResponseWriter, r *http.Request, scope string) bool {
	if !middleware.HasScope(r.Context(), scope) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("token missing required scope: %s", scope))
		return false
	}
	return true
}

// Deps bundles services needed by the public API handlers.
type Deps struct {
	Pool       *pgxpool.Pool
	Dispatcher *plugin.Dispatcher
	HoldSvc    *legalhold.Service
	Logger     *zap.Logger
}

// -----------------------------------------------------------------------
// Actions
// -----------------------------------------------------------------------

// ExecuteAction executes an action via bearer token.
// POST /api/v1/public/actions/execute  scope: integrations:execute
func (d *Deps) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	if !requireScope(w, r, "integrations:execute") {
		return
	}
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
	result, execErr := d.Dispatcher.Execute(r.Context(), instanceID, body.ActionKey, body.TargetEmail, body.Params)
	if execErr != nil {
		d.Logger.Error("public execute action", zap.Error(execErr))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("action failed: %s", execErr.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

// -----------------------------------------------------------------------
// Holds
// -----------------------------------------------------------------------

// ListHolds returns holds accessible via token.
// GET /api/v1/public/holds  scope: holds:read
func (d *Deps) ListHolds(w http.ResponseWriter, r *http.Request) {
	if !requireScope(w, r, "holds:read") {
		return
	}
	holds, err := d.HoldSvc.ListHolds(r.Context(), d.Pool, legalhold.ListHoldsFilter{})
	if err != nil {
		d.Logger.Error("public list holds", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holds": holds})
}

// CreateHold creates a hold via bearer token.
// POST /api/v1/public/holds  scope: holds:write
func (d *Deps) CreateHold(w http.ResponseWriter, r *http.Request) {
	if !requireScope(w, r, "holds:write") {
		return
	}
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

	claims, _ := middleware.TokenClaimsFromCtx(r.Context())
	actor := domain.Actor{Type: domain.ActorTypeToken}
	if claims != nil {
		actor.ID = claims.UserID
	}

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
		d.Logger.Error("public create hold", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = tx.Commit(r.Context())
	writeJSON(w, http.StatusCreated, map[string]any{"hold": hold})
}

// -----------------------------------------------------------------------
// Audit
// -----------------------------------------------------------------------

// ListAuditEvents returns audit events via token.
// GET /api/v1/public/audit/events  scope: audit:read
func (d *Deps) ListAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !requireScope(w, r, "audit:read") {
		return
	}
	rows, err := d.Pool.Query(r.Context(), `
		SELECT id, aggregate_type, aggregate_id, version, type, payload, actor_id, actor_type, created_at
		FROM events ORDER BY created_at DESC LIMIT 100
	`)
	if err != nil {
		d.Logger.Error("public list audit events", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var e domain.Event
		var aggTypeStr string
		var payloadBytes []byte
		var actorID *uuid.UUID
		var actorType *string
		if err := rows.Scan(&e.ID, &aggTypeStr, &e.AggregateID, &e.Version, &e.Type, &payloadBytes, &actorID, &actorType, &e.CreatedAt); err != nil {
			continue
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
		events = append(events, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// -----------------------------------------------------------------------
// Identities
// -----------------------------------------------------------------------

// SearchIdentities resolves an identity via token.
// GET /api/v1/public/identities/search?email=&instance_id=  scope: identities:read
func (d *Deps) SearchIdentities(w http.ResponseWriter, r *http.Request) {
	if !requireScope(w, r, "identities:read") {
		return
	}
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
		d.Logger.Error("public search identities", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity": identity})
}
