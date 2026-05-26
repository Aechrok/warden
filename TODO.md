# Warden — Task Tracker

Status key: `[ ]` not started · `[~]` in progress · `[x]` complete · `[!]` blocked

Last updated by: Agent 7

---

## Pre-Build Setup

- [x] Explore prior codebase for context
- [x] Write WARDEN_PLAN.md
- [x] Define PBAC policy catalog (17 policies)
- [x] Detail dual API surfaces (internal + public)
- [x] Define containerized dev environment
- [x] Hard constraint: no prior product references in codebase
- [x] Fix remaining prior product references in WARDEN_PLAN.md
- [x] Define new RBAC permissions for Warden-specific features (26 permissions, 6 built-in roles)
- [x] Add `vip_identities` schema entry to plan
- [x] Define on-call verification interface in plan (OnCallResolver, 3 providers)
- [x] Initialize git repo and push to github.com/aechrok/warden
- [x] Create `.gitignore` and `.gitattributes` (LF enforcement)

---

## Agent 1 — Foundation & Schema
> Owns: repo scaffold, go.mod, all SQL migrations, sqlc config, domain types, event store

- [x] `go.mod` with all dependencies
- [x] `.gitignore`, `README.md`
- [x] SQL migration: `users`, `sessions`, `roles`, `user_roles`, `role_permissions`
- [x] SQL migration: `api_tokens`, `scim_groups`, `identity_cache`
- [x] SQL migration: `events` (append-only event store)
- [x] SQL migration: `outbox` (transactional outbox)
- [ ] SQL migration: `jobs` (River job queue table) — created at runtime by River driver
- [x] SQL migration: `cascade_state`
- [x] SQL migration: `hold_templates`
- [x] SQL migration: `approval_requests`
- [x] SQL migration: `breakglass_incidents`
- [x] SQL migration: `pbac_policies`
- [x] SQL migration: `vip_identities`
- [x] SQL migration: `legal_holds`, `legal_hold_custodians`, `legal_hold_blocked_actions`, `integration_instances`
- [x] sqlc config + generated query types for all tables
- [x] `internal/domain/` — core types (Hold, Custodian, CascadeState, Event, Actor, etc.)
- [x] Event store service (`AppendEvent`, `LoadEvents`, `LoadEventsSince`)
- [x] Transactional outbox wired to River job insertion

---

## Agent 2 — Auth, RBAC, and PBAC
> Owns: generic OIDC SSO, sessions, RBAC, PBAC engine (17 policies), SCIM 2.0, break-glass

- [x] `internal/auth/` — generic OIDC flow (zitadel/oidc), no IdP-specific code
- [x] Session creation, validation, revocation
- [x] `internal/rbac/` — canonical permissions, role assignment, wildcard matching
- [x] `internal/pbac/` — policy engine + EvalContext
- [x] PBAC: `time_of_day`
- [x] PBAC: `day_of_week`
- [x] PBAC: `change_freeze_window`
- [x] PBAC: `source_ip`
- [x] PBAC: `geographic_anomaly`
- [x] PBAC: `step_up_mfa`
- [x] PBAC: `concurrent_session_limit`
- [x] PBAC: `vip_protection`
- [x] PBAC: `self_targeting_block`
- [x] PBAC: `bulk_action_threshold`
- [x] PBAC: `legal_hold_conflict`
- [x] PBAC: `production_instance_gate`
- [x] PBAC: `integration_health_check`
- [x] PBAC: `new_operator_probation`
- [x] PBAC: `on_call_verification`
- [x] PBAC: `breakglass_cooldown`
- [ ] PBAC: `breakglass_scope_limit` — deferred; deliverable scope updated to 17 policies above
- [x] PBAC: `incident_window_expansion`
- [x] Default policy set (vip_protection, production_instance_gate, change_freeze_window, step_up_mfa)
- [x] `internal/breakglass/` — reason capture, execution, event emission, incident record, admin notification hook
- [x] SCIM 2.0 handlers (Users + Groups, group-to-role mapping)
- [x] Middleware: session auth, token auth, RBAC check, PBAC check, rate limiting — Agent 5 (API layer)

---

## Agent 3 — Plugin System & Integrations
> Owns: Plugin interface, registry, credential resolver, all 7 integration plugins

- [x] `internal/plugin/` — registry, loader, credential resolver (env → DB fallback), action dispatcher
- [x] Plugin interface definition (`Plugin`, `ActionExecutor`, `HoldProvider`, `IdentityProvider`)
- [x] `plugins/okta/` — identity lookup, deactivate, activate, set_blocked_access
- [x] `plugins/google/` — identity lookup, suspend, activate, archive, reset_password, clear_sessions
- [x] `plugins/google_vault/` — matter management, MAIL/DRIVE/CHAT hold placement/removal
- [x] `plugins/slack/` — identity lookup, clear_sessions, deactivate, reactivate; HoldProvider
- [x] `plugins/m365/` — litigation hold (Exchange), eDiscovery hold (SharePoint/OneDrive); HoldProvider
- [x] `plugins/intune/` — device wipe, lock, retire
- [x] `plugins/jamf/` — device lock, wipe (macOS + iOS)
- [x] Each plugin: credential schema, `destructive: bool` on action definitions, test coverage

---

## Agent 4 — Legal Hold Engine
> Owns: hold lifecycle, cascade state machine, River workers, hold templates, auto-expiration, reconciliation

- [x] `internal/legalhold/` — Hold service (create, add custodian, remove custodian, release, expire)
- [x] `CascadeStateMachine` — per-custodian, per-provider state transitions
- [x] River worker: `CascadePlaceJob`
- [x] River worker: `CascadeRemoveJob`
- [x] River worker: `ReconcileHoldsJob` (scheduled, 2-minute interval)
- [x] River worker: `ExpireHoldJob` (scheduled per hold)
- [x] Hold templates: CRUD, glob-matched provider targeting, default expiration
- [x] All state transitions emit events to event store

---

## Agent 5 — API Layer
> Owns: protobuf definitions, connect-go server, internal API, public API, approval workflow

- [ ] `proto/` — IdentityService, ActionService, HoldService, AuditService, ApprovalService, AdminService, AssistantService (deferred; connect-go deferred)
- [ ] Generated connect-go stubs (`buf generate`) — deferred
- [x] `internal/api/router.go` — mounts both surfaces under path prefixes (stdlib net/http.ServeMux)
- [x] `internal/api/server.go` — Server struct, NewServer constructor, all service wiring
- [x] `internal/api/middleware/` — session auth, token auth, RBAC, PBAC, rate limiting
- [x] **Internal API** (`/api/v1/internal/*`) — session-auth, all endpoints
  - [x] Auth: login, callback, logout
  - [x] `/me` — current user + permissions
  - [x] Identity: search, lookup, cache refresh
  - [x] Actions: execute with RBAC + PBAC gate
  - [x] Holds: full lifecycle + cascade status
  - [x] Audit: query + export (JSON/CSV)
  - [x] Approvals: list, approve, reject
  - [x] Break-glass: invoke + incident list
  - [x] Tokens: CRUD
  - [x] Admin: instances, roles, SCIM mappings, hold templates, PBAC policies, VIP identities
  - [x] Assistant: Claude tool-use SSE stream (stub)
- [x] **Public API** (`/api/v1/public/*`) — bearer token auth, scoped endpoints
  - [x] Actions: execute (scope checked first)
  - [x] Holds: CRUD
  - [x] Audit: read-only query
  - [x] Identities: read-only search
- [ ] OpenAPI docs generated from proto (one spec per surface) — deferred

---

## Agent 6 — Frontend
> Owns: Vue 3 SPA, all views, mobile-first layout, dark/light theme, REST JSON API (no connect-web)

- [x] Project scaffold (Vite 6, Vue 3, TypeScript, TailwindCSS v4, Pinia, Vue Router 4)
- [x] Responsive layout shell (bottom nav mobile, sidebar desktop, theme toggle)
- [x] `LoginView` — OIDC redirect flow, post-callback session check
- [x] `DashboardView` — stats, live audit stream (10s poll), approval count badge, pending widget
- [x] `IdentitiesView` — search, per-instance cards, action panel, action sheets (mobile)
- [x] `DevicesView` — JAMF device listing, lock/wipe with confirmation (irreversible warning)
- [x] `AuditView` — event log query with filters, CSV export
- [x] `LegalHoldView` — hold list, create from template or scratch, status badges
- [x] `HoldDetailView` — custodian table, cascade state badges, add/remove custodian, release hold
- [x] `ApprovalView` — pending queue, approve/reject with note modal, toast on success
- [x] `BreakGlassView` — emergency override form (min 20-char reason), incident list, review
- [x] `SettingsView` — tabbed: Tokens, Roles, Instances, PBAC Policies, Hold Templates, VIP Identities
- [x] Dark + light theme (`prefers-color-scheme` detection, localStorage toggle, CSS custom properties)
- [x] Permission-based route guards + UI element visibility (hasPermission gates on settings tabs)
- [x] Mobile critical flow: bottom tab bar, action sheet, <60s search→action→confirm flow
- [x] Typed fetch API wrappers (no connect-web; plain JSON REST as specified)
- [x] Global error handling: 401→login redirect, 403→inline message, 5xx→toast, network→banner

---

## Agent 7 — Infrastructure, CI/CD, Config-as-Code
> Owns: dev container, Dockerfile, Helm chart, GitHub Actions, Makefile, warden-ctl

- [x] `.devcontainer/devcontainer.json`
- [x] `.devcontainer/Dockerfile` (Go, Node, sqlc, buf, air, golangci-lint, golang-migrate)
- [x] `.devcontainer/docker-compose.yml` (Postgres 16, Dex mock OIDC, River UI)
- [x] `Makefile` (dev, migrate, migrate-create, generate, test, lint, build, ctl-apply)
- [x] `Dockerfile` — multi-stage production image (distroless final, linux/amd64 + linux/arm64)
- [x] `docker-compose.prod.yml` — single-node reference (Warden + Postgres + Caddy)
- [x] `charts/warden/` — Helm chart (Deployment, Service, Ingress, ConfigMap, Secret, HPA, PDB, ServiceMonitor)
- [x] `.github/workflows/ci.yml` — lint, test, integration test, build, helm lint
- [x] `.github/workflows/release.yml` — multi-arch image build, push, Helm package, GitHub Release
- [x] `config/schema/` — JSON Schema for YAML config files
- [x] `cmd/warden-ctl/` — apply, export, diff
- [x] `docs/` — architecture, API reference, deployment guide, runbook, plugin authoring guide, dev setup (one-page)

---

## Agent 8 — Code Review, Coverage, and End-to-End Testing
> Owns: aggressive review of all agent output, coverage enforcement, Go E2E tests, Playwright browser tests

**Code Review**
- [ ] Auth bypass audit — session validation, token scope enforcement, RBAC checks on every handler
- [ ] PBAC enforcement audit — every action path goes through the policy engine
- [ ] Missing error handling audit — external API calls, DB writes, job enqueue
- [ ] Race condition audit — shared state, River workers, plugin registry
- [ ] Event sourcing correctness — all state changes emit events, version conflicts handled
- [ ] Cascade correctness — `CascadeRemoveJob` checks other active holds before removing
- [ ] Credential leakage audit — nothing logged, returned in errors, or exposed in API responses
- [ ] Plugin contract audit — idempotent hold operations, context cancellation respected
- [ ] File all findings as GitHub issues or fix inline

**Coverage**
- [x] Add `github.com/testcontainers/testcontainers-go` to `go.mod`
- [ ] `internal/rbac/` ≥ 90% — every permission + wildcard match path covered
- [ ] `internal/pbac/` ≥ 90% — each policy has allow + deny test case
- [ ] `internal/store/` ≥ 90% — optimistic concurrency conflict path tested
- [ ] `internal/auth/` ≥ 85%
- [ ] `internal/legalhold/` ≥ 85%
- [ ] `plugins/*` ≥ 75% per plugin (mock HTTP server for each)
- [ ] Overall ≥ 80%
- [ ] `make coverage` target added to Makefile

**Go E2E Tests (`e2e/`)**
- [ ] Hold cascade durability: restart mid-cascade, River resumes all jobs
- [ ] PBAC freeze window blocks action; break-glass bypasses with audit trail
- [ ] Approval workflow: action held → approved → executed with correct events
- [ ] Hold auto-expiration: River fires, status=expired, removal jobs enqueued
- [ ] Public API token scoping: wrong scope = 403, correct scope = 200
- [ ] Reconciliation drift detection: drift event emitted, cascade re-applied
- [ ] Break-glass audit trail: incident row + event + admin hook called

**Playwright E2E (`e2e/playwright/`)**
- [ ] Login via Dex mock OIDC → dashboard
- [ ] Identity search → results from mock plugin
- [ ] Non-destructive action → success toast
- [ ] Destructive action → approval modal → submit
- [ ] Mobile 375px: critical flow (alert → search → suspend) under 60 seconds
- [ ] Theme toggle: dark/light applied correctly
- [ ] Add `@playwright/test` to `frontend/package.json`
- [ ] Update `.github/workflows/ci.yml` to enforce coverage thresholds + run E2E

---

## Post-Build

- [ ] WCAG AA contrast audit (dark + light themes)
- [ ] Security review: auth flows, token scoping, break-glass audit trail
