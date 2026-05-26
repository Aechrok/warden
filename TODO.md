# Warden — Task Tracker

Status key: `[ ]` not started · `[~]` in progress · `[x]` complete · `[!]` blocked

Last updated by: Claude (pre-build complete)

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

- [ ] `go.mod` with all dependencies
- [ ] `.gitignore`, `README.md`
- [ ] SQL migration: `users`, `sessions`, `roles`, `user_roles`, `role_permissions`
- [ ] SQL migration: `api_tokens`, `scim_groups`, `identity_cache`
- [ ] SQL migration: `events` (append-only event store)
- [ ] SQL migration: `outbox` (transactional outbox)
- [ ] SQL migration: `jobs` (River job queue table)
- [ ] SQL migration: `cascade_state`
- [ ] SQL migration: `hold_templates`
- [ ] SQL migration: `approval_requests`
- [ ] SQL migration: `breakglass_incidents`
- [ ] SQL migration: `pbac_policies`
- [ ] SQL migration: `vip_identities`
- [ ] SQL migration: `legal_holds`, `legal_hold_custodians`, `legal_hold_blocked_actions`, `integration_instances`
- [ ] sqlc config + generated query types for all tables
- [ ] `internal/domain/` — core types (Hold, Custodian, CascadeState, Event, Actor, etc.)
- [ ] Event store service (`AppendEvent`, `LoadEvents`, `LoadEventsSince`)
- [ ] Transactional outbox wired to River job insertion

---

## Agent 2 — Auth, RBAC, and PBAC
> Owns: generic OIDC SSO, sessions, RBAC, PBAC engine (17 policies), SCIM 2.0, break-glass

- [ ] `internal/auth/` — generic OIDC flow (zitadel/oidc), no IdP-specific code
- [ ] Session creation, validation, revocation
- [ ] `internal/rbac/` — canonical permissions, role assignment, wildcard matching
- [ ] `internal/pbac/` — policy engine + EvalContext
- [ ] PBAC: `time_of_day`
- [ ] PBAC: `day_of_week`
- [ ] PBAC: `change_freeze_window`
- [ ] PBAC: `source_ip`
- [ ] PBAC: `geographic_anomaly`
- [ ] PBAC: `step_up_mfa`
- [ ] PBAC: `concurrent_session_limit`
- [ ] PBAC: `vip_protection`
- [ ] PBAC: `self_targeting_block`
- [ ] PBAC: `bulk_action_threshold`
- [ ] PBAC: `legal_hold_conflict`
- [ ] PBAC: `production_instance_gate`
- [ ] PBAC: `integration_health_check`
- [ ] PBAC: `new_operator_probation`
- [ ] PBAC: `on_call_verification`
- [ ] PBAC: `breakglass_cooldown`
- [ ] PBAC: `breakglass_scope_limit`
- [ ] PBAC: `incident_window_expansion`
- [ ] Default policy set (vip_protection, production_instance_gate, change_freeze_window, step_up_mfa)
- [ ] `internal/breakglass/` — reason capture, execution, event emission, incident record, admin notification hook
- [ ] SCIM 2.0 handlers (Users + Groups, group-to-role mapping)
- [ ] Middleware: session auth, token auth, RBAC check, PBAC check, rate limiting

---

## Agent 3 — Plugin System & Integrations
> Owns: Plugin interface, registry, credential resolver, all 7 integration plugins

- [ ] `internal/plugin/` — registry, loader, credential resolver (env → DB fallback), action dispatcher
- [ ] Plugin interface definition (`Plugin`, `ActionExecutor`, `HoldProvider`, `IdentityProvider`)
- [ ] `plugins/okta/` — identity lookup, deactivate, activate, set_blocked_access
- [ ] `plugins/google/` — identity lookup, suspend, activate, archive, reset_password, clear_sessions
- [ ] `plugins/google_vault/` — matter management, MAIL/DRIVE/CHAT hold placement/removal
- [ ] `plugins/slack/` — identity lookup, clear_sessions, deactivate, reactivate; HoldProvider
- [ ] `plugins/m365/` — litigation hold (Exchange), eDiscovery hold (SharePoint/OneDrive); HoldProvider
- [ ] `plugins/intune/` — device wipe, lock, retire
- [ ] `plugins/jamf/` — device lock, wipe (macOS + iOS)
- [ ] Each plugin: credential schema, `destructive: bool` on action definitions, test coverage

---

## Agent 4 — Legal Hold Engine
> Owns: hold lifecycle, cascade state machine, River workers, hold templates, auto-expiration, reconciliation

- [ ] `internal/legalhold/` — Hold service (create, add custodian, remove custodian, release, expire)
- [ ] `CascadeStateMachine` — per-custodian, per-provider state transitions
- [ ] River worker: `CascadePlaceJob`
- [ ] River worker: `CascadeRemoveJob`
- [ ] River worker: `ReconcileHoldsJob` (scheduled, 2-minute interval)
- [ ] River worker: `ExpireHoldJob` (scheduled per hold)
- [ ] Hold templates: CRUD, glob-matched provider targeting, default expiration
- [ ] All state transitions emit events to event store

---

## Agent 5 — API Layer
> Owns: protobuf definitions, connect-go server, internal API, public API, approval workflow

- [ ] `proto/` — IdentityService, ActionService, HoldService, AuditService, ApprovalService, AdminService, AssistantService
- [ ] Generated connect-go stubs (`buf generate`)
- [ ] `internal/api/router.go` — mounts both surfaces under path prefixes
- [ ] `internal/api/middleware/` — shared + surface-specific middleware
- [ ] **Internal API** (`/api/v1/internal/*`) — session-auth, all endpoints
  - [ ] Auth: login, callback, logout
  - [ ] `/me` — current user + permissions
  - [ ] Identity: search, lookup, cache refresh
  - [ ] Actions: execute with RBAC + PBAC gate
  - [ ] Holds: full lifecycle + cascade status
  - [ ] Audit: query + export (JSON/CSV)
  - [ ] Approvals: list, approve, reject
  - [ ] Break-glass: invoke + incident list
  - [ ] Tokens: CRUD
  - [ ] Admin: instances, roles, SCIM mappings, hold templates, PBAC policies
  - [ ] Assistant: Claude tool-use SSE stream
- [ ] **Public API** (`/api/v1/public/*`) — bearer token auth, scoped endpoints
  - [ ] Actions: execute (scope checked first)
  - [ ] Holds: CRUD
  - [ ] Audit: read-only query
  - [ ] Identities: read-only search
- [ ] OpenAPI docs generated from proto (one spec per surface)

---

## Agent 6 — Frontend
> Owns: Vue 3 SPA, all views, mobile-first layout, dark/light theme, connect-web

- [ ] Project scaffold (Vite, Vue 3, TypeScript, TailwindCSS v4, Pinia, Vue Router, connect-web)
- [ ] Responsive layout shell (bottom nav mobile, sidebar desktop, theme toggle)
- [ ] `LoginView` — OIDC redirect flow
- [ ] `DashboardView` — stats, live audit stream, approval count badge
- [ ] `IdentitiesView` — search, per-instance cards, action panel, action sheets (mobile)
- [ ] `DevicesView` — JAMF device listing, lock/wipe with approval gate
- [ ] `AuditView` — event log query, export
- [ ] `LegalHoldView` — hold list, create from template or scratch, custodian management, cascade status breakdown
- [ ] `ApprovalView` — pending queue, approve/reject with reason
- [ ] `BreakGlassView` — emergency override form, incident list
- [ ] `SettingsView` — roles, permissions, instances, SCIM, PBAC policies, hold templates, config export/import
- [ ] Dark + light theme (`prefers-color-scheme` detection, localStorage toggle)
- [ ] Permission-based route guards + UI element visibility
- [ ] Mobile critical flow: alert → search → suspend in under 60 seconds

---

## Agent 7 — Infrastructure, CI/CD, Config-as-Code
> Owns: dev container, Dockerfile, Helm chart, GitHub Actions, Makefile, warden-ctl

- [ ] `.devcontainer/devcontainer.json`
- [ ] `.devcontainer/Dockerfile` (Go, Node, sqlc, buf, air, golangci-lint, golang-migrate)
- [ ] `.devcontainer/docker-compose.yml` (Postgres 16, Dex mock OIDC, River UI)
- [ ] `Makefile` (dev, migrate, migrate-create, generate, test, lint, build, ctl-apply)
- [ ] `Dockerfile` — multi-stage production image (distroless final, linux/amd64 + linux/arm64)
- [ ] `docker-compose.prod.yml` — single-node reference (Warden + Postgres + Caddy)
- [ ] `charts/warden/` — Helm chart (Deployment, Service, Ingress, ConfigMap, Secret, HPA, PDB, ServiceMonitor)
- [ ] `.github/workflows/ci.yml` — lint, test, integration test, build, helm lint
- [ ] `.github/workflows/release.yml` — multi-arch image build, push, Helm package, GitHub Release
- [ ] `config/schema/` — JSON Schema for YAML config files
- [ ] `cmd/warden-ctl/` — apply, export, diff
- [ ] `docs/` — architecture, API reference, deployment guide, runbook, plugin authoring guide, dev setup (one-page)

---

## Post-Build

- [ ] End-to-end test: hold cascade survives server restart mid-job
- [ ] End-to-end test: PBAC default policies block correctly
- [ ] End-to-end test: mobile critical flow under 60 seconds
- [ ] WCAG AA contrast audit (dark + light themes)
- [ ] Security review: auth flows, token scoping, break-glass audit trail
