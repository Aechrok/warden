package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	apiinternal "github.com/aechrok/warden/internal/api/internal"
	"github.com/aechrok/warden/internal/api/middleware"
	apipublic "github.com/aechrok/warden/internal/api/public"
	"github.com/aechrok/warden/internal/api/docs"
	"github.com/aechrok/warden/internal/debug"
	"github.com/aechrok/warden/internal/rbac"
	"github.com/aechrok/warden/internal/scim"
)

// Handler builds and returns the root HTTP handler for the Warden server.
// It mounts:
//   - /auth/           — OIDC login + callback (no auth)
//   - /scim/v2/        — SCIM 2.0 (bearer token, scim:admin)
//   - /api/v1/internal — session cookie auth
//   - /api/v1/public   — bearer token auth
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	rl := middleware.NewRateLimiter(100)
	rateLimitMiddleware := rl.Middleware()

	sessionAuthMW := middleware.SessionAuth(s.sessions, s.users, s.pool)
	tokenAuthMW := middleware.TokenAuth(s.pool)

	authHandler := apiinternal.NewAuthHandler(s.auth, s.sessions, s.users, s.pool, s.logger, s.secureCookies())

	deps := &apiinternal.Deps{
		Pool:       s.pool,
		Sessions:   s.sessions,
		Users:      s.users,
		Checker:    s.checker,
		Dispatcher: s.dispatcher,
		HoldSvc:    s.holdSvc,
		BreakGlass: s.breakglass,
		EventStore: s.eventStore,
		Logger:     s.logger,
		Secure:     s.secureCookies(),
		EncKey:     s.cfg.EncryptionKey,
	}

	pubDeps := &apipublic.Deps{
		Pool:       s.pool,
		Dispatcher: s.dispatcher,
		HoldSvc:    s.holdSvc,
		Logger:     s.logger,
	}

	// ----------------------------------------------------------------
	// /auth/ — OIDC (unauthenticated)
	// ----------------------------------------------------------------
	mux.Handle("/auth/login", apply(http.HandlerFunc(authHandler.Login), rateLimitMiddleware))
	mux.Handle("/auth/callback", apply(http.HandlerFunc(authHandler.Callback), rateLimitMiddleware))
	mux.Handle("/auth/local", apply(http.HandlerFunc(authHandler.LocalLogin), rateLimitMiddleware))
	mux.Handle("/auth/magic", apply(http.HandlerFunc(authHandler.RedeemMagicLink), rateLimitMiddleware))
	mux.Handle("/auth/config", apply(http.HandlerFunc(authHandler.GetAuthConfig), rateLimitMiddleware))

	// ----------------------------------------------------------------
	// /scim/v2/ — SCIM 2.0 (bearer token + scim:admin)
	// ----------------------------------------------------------------
	mountSCIM(mux, s.pool, tokenAuthMW, rateLimitMiddleware)

	// ----------------------------------------------------------------
	// /api/v1/internal/ — session cookie auth
	// ----------------------------------------------------------------
	mountInternal(mux, deps, s, sessionAuthMW, rateLimitMiddleware)

	// ----------------------------------------------------------------
	// /api/v1/public/ — bearer token auth
	// ----------------------------------------------------------------
	mountPublic(mux, pubDeps, tokenAuthMW, rateLimitMiddleware)

	// ----------------------------------------------------------------
	// API docs (unauthenticated, rate-limited)
	// ----------------------------------------------------------------
	mux.Handle("/api/v1/internal/docs/", apply(http.StripPrefix("/api/v1/internal/docs", docs.InternalHandler()), rateLimitMiddleware))
	mux.Handle("/api/v1/public/docs/", apply(http.StripPrefix("/api/v1/public/docs", docs.PublicHandler()), rateLimitMiddleware))
	mux.Handle("/scim/v2/docs/", apply(http.StripPrefix("/scim/v2/docs", docs.SCIMHandler()), rateLimitMiddleware))

	// ----------------------------------------------------------------
	// / — Vue SPA (only when frontend/dist exists; no-op otherwise)
	// ----------------------------------------------------------------
	mountFrontend(mux)

	return mux
}

// apply chains middleware onto a handler (first = outermost).
func apply(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}


// requirePerm builds the RBAC middleware for a given permission.
func (s *Server) requirePerm(perm rbac.Permission) func(http.Handler) http.Handler {
	return middleware.RequirePermission(s.checker, s.pool, perm)
}

// requirePBAC builds the PBAC middleware.
func (s *Server) requirePBAC() func(http.Handler) http.Handler {
	return middleware.RequirePBAC(s.pbacEngine, s.pool, s.sessions, s.users, s.oncall)
}

// mountSCIM registers SCIM 2.0 handlers under /scim/v2/.
func mountSCIM(mux *http.ServeMux, pool *pgxpool.Pool, tokenAuth, rateLimitMW func(http.Handler) http.Handler) {
	uh := scim.NewUserHandler(pool)
	gh := scim.NewGroupHandler(pool)

	scimAuth := func(h http.Handler) http.Handler {
		return apply(h, tokenAuth, func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !middleware.HasScope(r.Context(), "scim:admin") {
					http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
	}

	scimUserH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Users/")
			if id != "" {
				u, err := uh.Get(r.Context(), id)
				if err != nil {
					scimError(w, scim.StatusForError(err), err.Error())
					return
				}
				writeScimJSON(w, http.StatusOK, u)
				return
			}
			q := r.URL.Query()
			startIndex := queryInt(q.Get("startIndex"), 1)
			count := queryInt(q.Get("count"), 50)
			resp, err := uh.List(r.Context(), startIndex, count)
			if err != nil {
				scimError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeScimJSON(w, http.StatusOK, resp)
		case http.MethodPost:
			var u scim.User
			if err := decodeScimBody(r, &u); err != nil {
				scimError(w, http.StatusBadRequest, err.Error())
				return
			}
			created, err := uh.Create(r.Context(), u)
			if err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			writeScimJSON(w, http.StatusCreated, created)
		case http.MethodPut:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Users/")
			var u scim.User
			if err := decodeScimBody(r, &u); err != nil {
				scimError(w, http.StatusBadRequest, err.Error())
				return
			}
			updated, err := uh.Update(r.Context(), id, u)
			if err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			writeScimJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Users/")
			if err := uh.Delete(r.Context(), id); err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	scimGroupH := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Groups/")
			if id != "" {
				g, err := gh.Get(r.Context(), id)
				if err != nil {
					scimError(w, scim.StatusForError(err), err.Error())
					return
				}
				writeScimJSON(w, http.StatusOK, g)
				return
			}
			q := r.URL.Query()
			resp, err := gh.List(r.Context(), queryInt(q.Get("startIndex"), 1), queryInt(q.Get("count"), 50))
			if err != nil {
				scimError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeScimJSON(w, http.StatusOK, resp)
		case http.MethodPost:
			var g scim.Group
			if err := decodeScimBody(r, &g); err != nil {
				scimError(w, http.StatusBadRequest, err.Error())
				return
			}
			created, err := gh.Create(r.Context(), g)
			if err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			writeScimJSON(w, http.StatusCreated, created)
		case http.MethodPut:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Groups/")
			var g scim.Group
			if err := decodeScimBody(r, &g); err != nil {
				scimError(w, http.StatusBadRequest, err.Error())
				return
			}
			updated, err := gh.Update(r.Context(), id, g)
			if err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			writeScimJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			id := extractLastSegment(r.URL.Path, "/scim/v2/Groups/")
			if err := gh.Delete(r.Context(), id); err != nil {
				scimError(w, scim.StatusForError(err), err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.Handle("/scim/v2/Users/", apply(scimAuth(scimUserH), rateLimitMW))
	mux.Handle("/scim/v2/Users", apply(scimAuth(scimUserH), rateLimitMW))
	mux.Handle("/scim/v2/Groups/", apply(scimAuth(scimGroupH), rateLimitMW))
	mux.Handle("/scim/v2/Groups", apply(scimAuth(scimGroupH), rateLimitMW))
}

// mountInternal registers all /api/v1/internal/* handlers.
func mountInternal(mux *http.ServeMux, deps *apiinternal.Deps, s *Server, sessionAuth, rateLimitMW func(http.Handler) http.Handler) {
	authMWs := []func(http.Handler) http.Handler{sessionAuth, rateLimitMW}

	// Auth (no session required)
	authHandler := apiinternal.NewAuthHandler(s.auth, s.sessions, s.users, s.pool, s.logger, s.secureCookies())
	mux.Handle("/api/v1/internal/auth/login", apply(http.HandlerFunc(authHandler.Login), rateLimitMW))
	mux.Handle("/api/v1/internal/auth/logout", apply(http.HandlerFunc(deps.Logout), sessionAuth, rateLimitMW))

	// Me
	mux.Handle("/api/v1/internal/me", apply(http.HandlerFunc(deps.Me), authMWs...))

	// Identities
	mux.Handle("/api/v1/internal/identities/search", apply(
		http.HandlerFunc(deps.SearchIdentities),
		append(authMWs, s.requirePerm(rbac.PermIdentitiesRead))...,
	))
	mux.Handle("/api/v1/internal/identities/cache/refresh", apply(
		http.HandlerFunc(deps.RefreshIdentityCache),
		append(authMWs, s.requirePerm(rbac.PermIdentitiesWrite))...,
	))

	// Actions
	mux.Handle("/api/v1/internal/actions/execute", apply(
		http.HandlerFunc(deps.ExecuteAction),
		append(authMWs, s.requirePerm(rbac.PermIntegrationsExec), s.requirePBAC())...,
	))
	mux.Handle("/api/v1/internal/actions/", apply(
		http.HandlerFunc(deps.ListActions),
		append(authMWs, s.requirePerm(rbac.PermIntegrationsRead))...,
	))

	// Holds — parameterised routes handled by the top-level dispatching handler.
	holdsH := holdsDispatch(deps, s)
	mux.Handle("/api/v1/internal/holds/", apply(holdsH, append(authMWs, s.requirePerm(rbac.PermHoldsRead))...))

	// Audit
	mux.Handle("/api/v1/internal/audit/events", apply(
		http.HandlerFunc(deps.ListAuditEvents),
		append(authMWs, s.requirePerm(rbac.PermAuditRead))...,
	))
	mux.Handle("/api/v1/internal/audit/export", apply(
		http.HandlerFunc(deps.ExportAuditEvents),
		append(authMWs, s.requirePerm(rbac.PermAuditRead))...,
	))

	// Approvals
	approvalsH := approvalsDispatch(deps, s)
	mux.Handle("/api/v1/internal/approvals/", apply(approvalsH, append(authMWs, s.requirePerm(rbac.PermApprovalsRead))...))

	// Break-glass
	mux.Handle("/api/v1/internal/breakglass/invoke", apply(
		http.HandlerFunc(deps.InvokeBreakGlass),
		append(authMWs, s.requirePerm(rbac.PermBreakGlassUse))...,
	))
	bgIncidentsH := bgIncidentsDispatch(deps, s)
	mux.Handle("/api/v1/internal/breakglass/incidents/", apply(bgIncidentsH, append(authMWs, s.requirePerm(rbac.PermBreakGlassReview))...))
	mux.Handle("/api/v1/internal/breakglass/incidents", apply(
		http.HandlerFunc(deps.ListIncidents),
		append(authMWs, s.requirePerm(rbac.PermBreakGlassReview))...,
	))

	// Tokens
	tokensH := tokensDispatch(deps, s)
	mux.Handle("/api/v1/internal/tokens/", apply(tokensH, append(authMWs, s.requirePerm(rbac.PermTokensRead))...))
	mux.Handle("/api/v1/internal/tokens", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListTokens(w, r)
			case http.MethodPost:
				deps.CreateToken(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}),
		append(authMWs, s.requirePerm(rbac.PermTokensRead))...,
	))

	// Admin
	mountAdmin(mux, deps, s, authMWs)

	// Assistant
	mux.Handle("/api/v1/internal/assistant/stream", apply(
		http.HandlerFunc(deps.AssistantStream),
		append(authMWs, s.requirePerm(rbac.PermAssistantUse))...,
	))

	// Debug stats (only when tracer is active — requires WARDEN_DEBUG=true at startup)
	if s.debugTracer != nil {
		mux.Handle("/api/v1/internal/debug/stats", apply(
			http.HandlerFunc(debug.StatsHandler(s.debugTracer)),
			authMWs...,
		))
	}
}

// mountPublic registers all /api/v1/public/* handlers.
func mountPublic(mux *http.ServeMux, deps *apipublic.Deps, tokenAuth, rateLimitMW func(http.Handler) http.Handler) {
	base := []func(http.Handler) http.Handler{tokenAuth, rateLimitMW}

	mux.Handle("/api/v1/public/actions/execute", apply(http.HandlerFunc(deps.ExecuteAction), base...))

	// Holds — exact paths must be registered before the wildcard /holds/ prefix.
	mux.Handle("/api/v1/public/holds/check", apply(http.HandlerFunc(deps.IsOnHold), base...))
	mux.Handle("/api/v1/public/holds", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListHolds(w, r)
			case http.MethodPost:
				deps.CreateHold(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}), base...))
	mux.Handle("/api/v1/public/holds/", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := pathSegments(r.URL.Path, "/api/v1/public/holds/")
			switch {
			case len(parts) == 1:
				if r.Method == http.MethodGet {
					deps.GetHold(w, r, parts[0])
				} else {
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			case len(parts) == 2 && parts[1] == "custodians" && r.Method == http.MethodPost:
				deps.AddCustodian(w, r, parts[0])
			case len(parts) == 3 && parts[1] == "custodians" && r.Method == http.MethodDelete:
				deps.RemoveCustodian(w, r, parts[0], parts[2])
			case len(parts) == 2 && parts[1] == "release" && r.Method == http.MethodPost:
				deps.ReleaseHold(w, r, parts[0])
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}), base...))

	mux.Handle("/api/v1/public/hold-templates", apply(http.HandlerFunc(deps.ListHoldTemplates), base...))
	mux.Handle("/api/v1/public/audit/events", apply(http.HandlerFunc(deps.ListAuditEvents), base...))
	mux.Handle("/api/v1/public/identities/search", apply(http.HandlerFunc(deps.SearchIdentities), base...))
}

// mountAdmin registers admin sub-routes.
func mountAdmin(mux *http.ServeMux, deps *apiinternal.Deps, s *Server, authMWs []func(http.Handler) http.Handler) {
	// Plugins
	mux.Handle("/api/v1/internal/admin/plugins", apply(
		http.HandlerFunc(deps.ListPlugins),
		append(authMWs, s.requirePerm(rbac.PermInstancesRead))...,
	))

	// Instances
	instancesH := instancesDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/instances/", apply(instancesH, append(authMWs, s.requirePerm(rbac.PermInstancesRead))...))
	mux.Handle("/api/v1/internal/admin/instances", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListInstances(w, r)
			case http.MethodPost:
				deps.CreateInstance(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}), append(authMWs, s.requirePerm(rbac.PermInstancesRead))...))

	// Users
	mux.Handle("/api/v1/internal/admin/users", apply(
		http.HandlerFunc(deps.ListUsers),
		append(authMWs, s.requirePerm(rbac.PermUsersRead))...,
	))
	mux.Handle("/api/v1/internal/admin/users/", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/users/")
			if len(parts) == 2 && parts[1] == "password" && r.Method == http.MethodPut {
				s.requirePerm(rbac.PermUsersWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.SetUserPassword(w, r, parts[0])
				})).ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}),
		append(authMWs, s.requirePerm(rbac.PermUsersRead))...,
	))

	// Invitations (magic links)
	invitationsH := invitationsDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/invitations/", apply(invitationsH, append(authMWs, s.requirePerm(rbac.PermUsersRead))...))
	mux.Handle("/api/v1/internal/admin/invitations", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListInvitations(w, r)
			case http.MethodPost:
				s.requirePerm(rbac.PermUsersWrite)(http.HandlerFunc(deps.CreateInvitation)).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}),
		append(authMWs, s.requirePerm(rbac.PermUsersRead))...,
	))

	// Permissions catalog
	mux.Handle("/api/v1/internal/admin/permissions", apply(
		http.HandlerFunc(deps.ListPermissions),
		append(authMWs, s.requirePerm(rbac.PermRolesRead))...,
	))

	// Roles
	rolesH := rolesDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/roles/", apply(rolesH, append(authMWs, s.requirePerm(rbac.PermRolesRead))...))
	mux.Handle("/api/v1/internal/admin/roles", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListRoles(w, r)
			case http.MethodPost:
				s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(deps.CreateRole)).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}),
		append(authMWs, s.requirePerm(rbac.PermRolesRead))...,
	))

	// PBAC
	pbacH := pbacDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/pbac/", apply(pbacH, append(authMWs, s.requirePerm(rbac.PermPBACRead))...))
	mux.Handle("/api/v1/internal/admin/pbac", apply(http.HandlerFunc(deps.ListPBACPolicies), append(authMWs, s.requirePerm(rbac.PermPBACRead))...))

	// Hold templates
	htH := holdTemplatesDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/hold-templates/", apply(htH, append(authMWs, s.requirePerm(rbac.PermHoldTemplatesRead))...))
	mux.Handle("/api/v1/internal/admin/hold-templates", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListHoldTemplates(w, r)
			case http.MethodPost:
				deps.CreateHoldTemplate(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}), append(authMWs, s.requirePerm(rbac.PermHoldTemplatesRead))...))

	// SSO Config
	mux.Handle("/api/v1/internal/admin/sso-config", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.GetSSOConfig(w, r)
			case http.MethodPut:
				s.requirePerm(rbac.PermInstancesWrite)(http.HandlerFunc(deps.UpdateSSOConfig)).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}),
		append(authMWs, s.requirePerm(rbac.PermInstancesRead))...,
	))

	// SCIM Groups
	scimGroupsH := scimGroupsDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/scim-groups/", apply(scimGroupsH, append(authMWs, s.requirePerm(rbac.PermRolesRead))...))
	mux.Handle("/api/v1/internal/admin/scim-groups", apply(
		http.HandlerFunc(deps.ListSCIMGroups),
		append(authMWs, s.requirePerm(rbac.PermRolesRead))...,
	))

	// VIP
	vipH := vipDispatch(deps, s)
	mux.Handle("/api/v1/internal/admin/vip/", apply(vipH, append(authMWs, s.requirePerm(rbac.PermVIPRead))...))
	mux.Handle("/api/v1/internal/admin/vip", apply(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				deps.ListVIP(w, r)
			case http.MethodPost:
				deps.AddVIP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}), append(authMWs, s.requirePerm(rbac.PermVIPRead))...))
}

// The following dispatch functions map URL path suffixes to parameterised handler calls.
// stdlib ServeMux doesn't do path parameters so we extract them manually.

func holdsDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/v1/internal/holds/{id}
		// /api/v1/internal/holds/{id}/custodians
		// /api/v1/internal/holds/{id}/custodians/{custodianId}
		// /api/v1/internal/holds/{id}/release
		// /api/v1/internal/holds/ (list/create)
		path := r.URL.Path
		parts := pathSegments(path, "/api/v1/internal/holds/")
		switch {
		case len(parts) == 0:
			switch r.Method {
			case http.MethodGet:
				deps.ListHolds(w, r)
			case http.MethodPost:
				requireWriteMW := s.requirePerm(rbac.PermHoldsWrite)
				requireWriteMW(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.CreateHold(w, r)
				})).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case len(parts) == 1:
			if r.Method == http.MethodGet {
				deps.GetHold(w, r, parts[0])
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case len(parts) == 2 && parts[1] == "custodians":
			if r.Method == http.MethodPost {
				requireWrite(w, r, s, func() { deps.AddCustodian(w, r, parts[0]) })
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case len(parts) == 3 && parts[1] == "custodians":
			if r.Method == http.MethodDelete {
				requireWrite(w, r, s, func() { deps.RemoveCustodian(w, r, parts[0], parts[2]) })
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case len(parts) == 2 && parts[1] == "release":
			if r.Method == http.MethodPost {
				requireWrite(w, r, s, func() { deps.ReleaseHold(w, r, parts[0]) })
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func requireWrite(w http.ResponseWriter, r *http.Request, s *Server, fn func()) {
	s.requirePerm(rbac.PermHoldsWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn()
	})).ServeHTTP(w, r)
}

func approvalsDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/approvals/")
		switch {
		case len(parts) == 0:
			deps.ListApprovals(w, r)
		case len(parts) == 2 && parts[1] == "approve":
			s.requirePerm(rbac.PermApprovalsWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.ApproveRequest(w, r, parts[0])
			})).ServeHTTP(w, r)
		case len(parts) == 2 && parts[1] == "reject":
			s.requirePerm(rbac.PermApprovalsWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.RejectRequest(w, r, parts[0])
			})).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func bgIncidentsDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/breakglass/incidents/")
		if len(parts) == 2 && parts[1] == "review" {
			deps.ReviewIncident(w, r, parts[0])
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func tokensDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/tokens/")
		if len(parts) == 1 && r.Method == http.MethodDelete {
			s.requirePerm(rbac.PermTokensWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.RevokeToken(w, r, parts[0])
			})).ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func instancesDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/instances/")
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodPut:
				s.requirePerm(rbac.PermInstancesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.UpdateInstance(w, r, parts[0])
				})).ServeHTTP(w, r)
			case http.MethodDelete:
				s.requirePerm(rbac.PermInstancesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.DeleteInstance(w, r, parts[0])
				})).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func rolesDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/v1/internal/admin/roles/{name}
		// /api/v1/internal/admin/roles/{name}/assign
		// /api/v1/internal/admin/roles/{name}/permissions
		// /api/v1/internal/admin/roles/{name}/users/{userId}
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/roles/")
		switch {
		case len(parts) == 1 && r.Method == http.MethodDelete:
			s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.DeleteRole(w, r, parts[0])
			})).ServeHTTP(w, r)
		case len(parts) == 2 && parts[1] == "assign" && r.Method == http.MethodPost:
			s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.AssignRole(w, r, parts[0])
			})).ServeHTTP(w, r)
		case len(parts) == 2 && parts[1] == "permissions" && r.Method == http.MethodPut:
			s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.UpdateRolePermissions(w, r, parts[0])
			})).ServeHTTP(w, r)
		case len(parts) == 3 && parts[1] == "users" && r.Method == http.MethodDelete:
			s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.RevokeRole(w, r, parts[0], parts[2])
			})).ServeHTTP(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func pbacDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/pbac/")
		if len(parts) == 1 && r.Method == http.MethodPut {
			s.requirePerm(rbac.PermPBACWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.UpdatePBACPolicy(w, r, parts[0])
			})).ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func holdTemplatesDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/hold-templates/")
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodPut:
				s.requirePerm(rbac.PermHoldTemplatesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.UpdateHoldTemplate(w, r, parts[0])
				})).ServeHTTP(w, r)
			case http.MethodDelete:
				s.requirePerm(rbac.PermHoldTemplatesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					deps.DeleteHoldTemplate(w, r, parts[0])
				})).ServeHTTP(w, r)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func scimGroupsDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /api/v1/internal/admin/scim-groups/{id}/role
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/scim-groups/")
		if len(parts) == 2 && parts[1] == "role" && r.Method == http.MethodPut {
			s.requirePerm(rbac.PermRolesWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.UpdateSCIMGroupRole(w, r, parts[0])
			})).ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func invitationsDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/invitations/")
		if len(parts) == 1 && r.Method == http.MethodDelete {
			s.requirePerm(rbac.PermUsersWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.DeleteInvitation(w, r, parts[0])
			})).ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func vipDispatch(deps *apiinternal.Deps, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := pathSegments(r.URL.Path, "/api/v1/internal/admin/vip/")
		if len(parts) == 1 && r.Method == http.MethodDelete {
			s.requirePerm(rbac.PermVIPWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.RemoveVIP(w, r, parts[0])
			})).ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// pathSegments strips prefix and splits remaining path into non-empty segments.
func pathSegments(path, prefix string) []string {
	tail := strings.TrimPrefix(path, prefix)
	tail = strings.Trim(tail, "/")
	if tail == "" {
		return nil
	}
	return strings.Split(tail, "/")
}

func extractLastSegment(path, prefix string) string {
	parts := pathSegments(path, prefix)
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

func queryInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func writeScimJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func scimError(w http.ResponseWriter, status int, detail string) {
	writeScimJSON(w, status, scim.NewError(status, detail))
}

func decodeScimBody(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// mountFrontend serves the built Vue SPA from frontend/dist when that
// directory is present. Unknown paths fall through to index.html so that
// client-side routing works. When the directory is absent (devcontainer,
// unit tests) the handler is simply not registered and the mux returns 404
// for unmatched paths.
func mountFrontend(mux *http.ServeMux) {
	const dist = "frontend/dist"
	if _, err := os.Stat(dist); os.IsNotExist(err) {
		return
	}
	fs := http.FileServer(http.Dir(dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dist, filepath.Clean("/"+r.URL.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(dist, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}
