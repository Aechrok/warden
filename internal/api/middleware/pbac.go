package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/pbac"
)

// Context keys for PBAC action metadata. Handlers that need PBAC evaluation
// set these in the context before registering the handler under this middleware.
type pbacActionKeyCtxKey struct{}
type pbacDestructiveCtxKey struct{}
type pbacInstanceNameCtxKey struct{}

// WithPBACMeta returns a context annotated with the action metadata used to
// build the EvalContext.
func WithPBACMeta(ctx context.Context, actionKey string, destructive bool, instanceName string) context.Context {
	ctx = context.WithValue(ctx, pbacActionKeyCtxKey{}, actionKey)
	ctx = context.WithValue(ctx, pbacDestructiveCtxKey{}, destructive)
	ctx = context.WithValue(ctx, pbacInstanceNameCtxKey{}, instanceName)
	return ctx
}

// RequirePBAC evaluates the PBAC policy engine for the incoming request.
//   - Allow / Override → call next
//   - RequireApproval  → INSERT approval_requests row, return HTTP 202
//   - Deny             → return HTTP 403
func RequirePBAC(engine *pbac.Engine, pool *pgxpool.Pool, sessions *auth.SessionStore, users *auth.UserStore, resolver pbac.OnCallResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Peek at the body so we can extract target_email without consuming it.
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			targetEmail := extractTargetEmail(r, bodyBytes)

			// Resolve actor identity.
			var userID uuid.UUID
			var userEmail string
			if u, ok := UserFromCtx(ctx); ok {
				userID = u.ID
				userEmail = u.Email
			} else if claims, ok := TokenClaimsFromCtx(ctx); ok {
				userID = claims.UserID
			}

			// Session age from the stored session.
			var sessionAge time.Duration
			if sd, ok := SessionFromCtx(ctx); ok {
				sessionAge = time.Since(sd.CreatedAt)
			}

			// Number of active sessions for this user.
			sessionCount := 0
			if sessions != nil && userID != uuid.Nil {
				n, err := sessions.CountActiveForUser(ctx, pool, userID)
				if err == nil {
					sessionCount = n
				}
			}

			// How long the user's account has existed.
			var operatorTenure time.Duration
			if users != nil && userID != uuid.Nil {
				d, err := users.Tenure(ctx, pool, userID)
				if err == nil {
					operatorTenure = d
				}
			}

			// On-call status; default true so a resolver error doesn't block the operator.
			onCall := true
			if resolver != nil && userEmail != "" {
				oc, err := resolver.IsOnCall(ctx, userEmail)
				if err == nil {
					onCall = oc
				}
			}

			// VIP check.
			targetIsVIP := false
			if targetEmail != "" {
				var count int
				_ = pool.QueryRow(ctx, `
					SELECT COUNT(*)::int FROM vip_identities WHERE email = $1
				`, strings.ToLower(targetEmail)).Scan(&count)
				targetIsVIP = count > 0
			}

			targetIsSelf := userEmail != "" && strings.EqualFold(targetEmail, userEmail)

			// Action metadata injected by the handler.
			actionKey, _ := ctx.Value(pbacActionKeyCtxKey{}).(string)
			destructive, _ := ctx.Value(pbacDestructiveCtxKey{}).(bool)
			instanceName, _ := ctx.Value(pbacInstanceNameCtxKey{}).(string)

			ec := pbac.EvalContext{
				Now:            time.Now().UTC(),
				SourceIP:       extractSourceIP(r),
				SessionAge:     sessionAge,
				SessionCount:   sessionCount,
				OperatorTenure: operatorTenure,
				OnCall:         onCall,
				TargetEmail:    targetEmail,
				TargetIsVIP:    targetIsVIP,
				TargetIsSelf:   targetIsSelf,
				InstanceName:   instanceName,
				ActionKey:      actionKey,
				Destructive:    destructive,
			}

			result := engine.Evaluate(ctx, ec)

			switch result {
			case pbac.Allow, pbac.Override:
				next.ServeHTTP(w, r)
			case pbac.RequireApproval:
				approvalID, err := createApprovalRequest(ctx, pool, userID, actionKey, bodyBytes, targetEmail)
				if err != nil {
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"status":      "pending_approval",
					"approval_id": approvalID.String(),
				})
			default:
				http.Error(w, `{"error":"action denied by policy"}`, http.StatusForbidden)
			}
		})
	}
}

// createApprovalRequest inserts a pending approval row and returns its UUID.
func createApprovalRequest(ctx context.Context, pool *pgxpool.Pool, requesterID uuid.UUID, actionKey string, bodyBytes []byte, targetEmail string) (uuid.UUID, error) {
	// Extract params and instance_id from the body if present.
	params := json.RawMessage(`{}`)
	var instanceID *uuid.UUID

	if len(bodyBytes) > 0 {
		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err == nil {
			if p, ok := body["params"]; ok {
				if pb, err := json.Marshal(p); err == nil {
					params = pb
				}
			}
			if raw, ok := body["instance_id"].(string); ok {
				if id, err := uuid.Parse(raw); err == nil {
					instanceID = &id
				}
			}
		}
	}

	var te any
	if targetEmail != "" {
		te = strings.ToLower(targetEmail)
	}

	var approvalID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO approval_requests (requester_id, action_key, instance_id, target_email, params, expires_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, now() + interval '24 hours')
		RETURNING id
	`, requesterID, actionKey, instanceID, te, string(params)).Scan(&approvalID)
	if err != nil {
		return uuid.Nil, err
	}
	return approvalID, nil
}

func extractTargetEmail(r *http.Request, bodyBytes []byte) string {
	if v := r.URL.Query().Get("email"); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	if v := r.URL.Query().Get("target_email"); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	if len(bodyBytes) > 0 {
		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err == nil {
			if v, ok := body["target_email"].(string); ok && v != "" {
				return strings.ToLower(strings.TrimSpace(v))
			}
			if v, ok := body["email"].(string); ok && v != "" {
				return strings.ToLower(strings.TrimSpace(v))
			}
		}
	}
	return ""
}

func extractSourceIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}
