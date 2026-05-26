# Warden: Full Rebuild Plan

> A new product built from scratch. Informed by prior work; not derived from it.

---

## Hard Constraints

These apply to every agent, every file, every line of code:

1. **No references to Dominion in the codebase.** Warden is a separate product. The name "Dominion" must not appear anywhere in source code, comments, commit messages, documentation, config files, error messages, log output, or test fixtures. This plan document uses the name for context only — agents must not carry it into implementation.
2. **No Windows-specific code, paths, or assumptions.** All tooling runs inside Linux containers. See the Development Environment section.
3. **No Okta-specific code outside `plugins/okta/`.** Auth is generic OIDC. Okta is one plugin among many.

---

## Background: The Problem Warden Solves

Warden is a centralized IT command-and-control platform. It gives security and IT operations teams a unified interface to:

- **Search identities** across connected systems (Okta, Google Workspace, Slack, JAMF, M365, Intune) without storing identity data
- **Execute authorized actions** (suspend accounts, wipe devices, clear sessions, reset passwords) with RBAC gates and full audit trail
- **Enforce legal holds** that cascade to downstream systems and stay synchronized
- **Audit everything** — every action, hold placement, and config change is logged and exportable

Without Warden, operators must log into each system separately to perform actions. With Warden, everything is in one place, governed by roles, tracked completely, and enforced by holds.

**Target tech stack:** Go backend, connect-go (gRPC + REST), sqlc for type-safe queries, PostgreSQL 16+, River job queue, Vue 3 + TypeScript + TailwindCSS frontend, generic OIDC SSO, Helm chart for Kubernetes.

---

## Design Decisions: What Warden Does Differently

> This section references prior art for planning context only. None of these references belong in the codebase. Agents read this section to understand *why* design decisions were made, not to port or adapt existing code.

These are the concrete decisions that shape Warden's architecture, in priority order:

### 1. Generic OIDC (not Okta-only)
Dominion hardcodes Okta. Warden is provider-agnostic: configure issuer, client_id, client_secret, scopes. Works with Authentik, Azure AD, Keycloak, Okta, or any OIDC-compliant IdP. Zero IdP-specific code in the auth layer.

### 2. Event Sourcing over Audit Logs
Dominion has an `audit_logs` table — append-only rows, good for queries, but no state machine. Warden uses an `events` table with `aggregate_type`, `aggregate_id`, and `version`. Every state change is an event. Projections build read models. You can replay, time-travel, and recover from partial failures. Legal hold cascades are complex state machines — event sourcing gives audit, debug, and recovery for free.

### 3. Durable Job Queue (River) over Fire-and-Forget Goroutines
Dominion runs cascade operations in `context.Background()` goroutines. Server restart mid-cascade = lost work, no retry, no visibility. Warden uses River (PostgreSQL-backed job queue). Each custodian-provider pair is a separate durable job. Jobs survive restarts, retry with exponential backoff, and dead-letter on final failure. "We tried but the goroutine died" is not an acceptable audit response for legal holds.

### 4. Formal Cascade State Machine
Dominion has a `legal_hold_cascade_results` table but no state machine. Partial failures are logged but not retryable. Warden has a formal state per custodian (`pending → in_progress → completed | partial | failed`) and per provider step (`pending → in_progress → completed | failed`). Failed steps can be retried individually without re-running the full cascade.

### 5. Plugin Architecture
Dominion hardcodes adapters in `internal/integrations/`. Adding a new integration means modifying core. Warden defines a `Plugin` interface: `RegisterActions()`, `RegisterCredentialSchema()`, `NewActionExecutor()`, `HoldProvider()`, `IdentityProvider()`. Plugins register via `init()`. New integrations live under `plugins/` and are imported — the core never touches plugin code. Third parties can write integrations without forking.

### 6. Approval Workflows for Destructive Actions
Dominion executes immediately. One operator with the right permission can wipe a CEO's device with no oversight. Warden: actions tagged `destructive: true` can require approval from a second operator. Approval requests have configurable timeouts (auto-reject after N hours). Approvers see a dedicated queue. All approval events are in the event store.

### 7. Break-Glass Procedure
Dominion: if RBAC blocks you, you're blocked. Warden: break-glass allows emergency override. Operator provides a reason. Action executes immediately. `breakglass` event is emitted. A post-incident review is tracked in a dedicated table. Admins are notified. This is how real incident response works at 3 AM.

### 8. PBAC Layer on Top of RBAC
Dominion is pure RBAC. If you have `okta-prod:user:deactivate`, you can deactivate anyone, anytime, from any IP. Warden adds PBAC: policies evaluate runtime context and emit `allow`, `deny`, or `require_approval`. Policies are YAML-defined and loaded declaratively. The full policy catalog:

**Time & Schedule**
- `time_of_day` — deny or require approval outside configured hours (e.g., block destructive actions after 6 PM)
- `day_of_week` — deny on weekends or configured non-business days
- `change_freeze_window` — deny all non-emergency writes during configured freeze periods (release windows, holidays); break-glass still permitted

**Network & Session**
- `source_ip` — deny if operator IP is outside an allowlist (e.g., corporate VPN CIDR); block or require approval
- `geographic_anomaly` — deny if operator's IP geolocates outside their configured operating region
- `step_up_mfa` — require re-authentication for destructive actions if session is older than N minutes
- `concurrent_session_limit` — deny if operator has more than N active sessions (compromise indicator)

**Target Sensitivity**
- `vip_protection` — deny actions on flagged identities (C-suite, board, legal counsel) without a second approver, regardless of role
- `self_targeting_block` — deny any action where the acting operator is the target identity
- `bulk_action_threshold` — deny if the same action is applied to more than N identities within a rolling time window (anomaly: mass deactivation)
- `legal_hold_conflict` — deny releasing a hold if the custodian is referenced in an active litigation matter

**Environment**
- `production_instance_gate` — destructive actions on instances matching `*-prod` require approval; same action on `*-staging` executes immediately
- `integration_health_check` — deny action if the target integration's last health check failed (prevents partial execution against a degraded system)

**Operator Posture**
- `new_operator_probation` — restrict destructive actions for operators whose accounts were created within the last N days
- `on_call_verification` — high-impact actions (device wipe, mass deactivation) only permitted if operator is on the current on-call roster (sourced via PagerDuty/OpsGenie integration)

**Break-Glass Controls**
- `breakglass_cooldown` — deny a second break-glass invocation from the same operator within N hours of the first (forces escalation instead of repeated bypassing)
- `breakglass_scope_limit` — break-glass unlocks only the specific action requested, not all PBAC policies for the session

**Operational Safety**
- `incident_window_expansion` — temporarily expand permissions for operators during an active declared incident; incident is declared via API, is time-bounded, and all expansions are logged as events

The four must-have policies for enterprise sales are: `vip_protection`, `production_instance_gate`, `change_freeze_window`, and `step_up_mfa`. A default policy set shipping with Warden covers all four with sane defaults that can be overridden.

### 9. Hold Templates
Dominion: every hold is created from scratch. Warden: hold templates define reusable configurations — which providers to cascade to (glob-matched on instance names), which actions to block, default expiration, notes templates. Creating a hold from a template pre-fills everything. Legal teams place the same hold type repeatedly; templates reduce errors.

### 10. Hold Expiration with Auto-Release
Dominion holds exist until manually released. No expiration. Warden: optional `expires_at` on holds. A River scheduled job auto-releases on expiration, including cascading the removal to all downstream systems. Templates can set default expiration days.

### 11. Continuous Reconciliation (2-minute, drift-evented)
Dominion reconciler runs every 10 minutes, goroutine-based. Warden: River scheduled job runs every 2 minutes (configurable). Per-provider reconciliation with drift detection. Drift events are emitted to the event store for audit. 10 minutes is a long window for a hold to be accidentally lifted.

### 12. gRPC + REST via connect-go
Dominion is pure REST (chi router). Warden uses connect-go: gRPC and REST from the same protobuf definitions. Internal service-to-service communication uses gRPC. External API is REST (auto-generated from proto). Frontend can use either. The proto definitions serve as living API documentation.

### 13. Mobile-First Frontend
Dominion is desktop-first. Warden: mobile-first responsive design, three breakpoints, bottom nav on mobile, action sheets instead of modals. The critical flow — receive alert, search identity, execute suspend — works on a phone in under 60 seconds. On-call engineers use their phones.

### 14. Dark + Light Theme
Dominion: dark only. Warden: dark (default) and light, respects `prefers-color-scheme` on first visit, theme toggle in header. Accessibility and enterprise sales checkbox.

### 15. Configuration as Code
Dominion: all config via UI/API. Warden: UI/API plus declarative YAML/JSON. Roles, permissions, policies, SCIM mappings, and integration configs can be defined in YAML and applied declaratively. GitOps-friendly — review config changes in PRs, roll back via git, replicate across environments.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────┐
│                  Warden Frontend                │
│        Vue 3 · TypeScript · TailwindCSS         │
│     Mobile-first · Dark/Light · connect-web     │
└─────────────────────┬───────────────────────────┘
                      │ gRPC-Web / REST
┌─────────────────────▼───────────────────────────┐
│                 Warden API Server               │
│   connect-go (gRPC + REST) · Generic OIDC SSO  │
│   RBAC + PBAC · Approval Queue · Break-Glass   │
└──────┬──────────────┬──────────────┬────────────┘
       │              │              │
┌──────▼──────┐ ┌─────▼─────┐ ┌─────▼──────────┐
│  Event Store│ │ River Jobs │ │  Plugin Registry│
│  (Postgres) │ │ (Postgres) │ │  (init() hooks) │
└─────────────┘ └─────────────┘ └────────────────┘
                                        │
                      ┌─────────────────┼──────────────────────┐
                      │                 │                      │
               ┌──────▼─────┐  ┌───────▼────┐  ┌─────────────▼────┐
               │  plugins/  │  │  plugins/  │  │    plugins/       │
               │    okta    │  │   google   │  │    slack / m365   │
               │   jamf     │  │   vault    │  │    intune / ...   │
               └────────────┘  └────────────┘  └──────────────────┘
```

---

## Database Schema

### Remove
- `audit_logs` table (replaced by event store)
- `legal_hold_cascade_results` table (replaced by cascade events in event store)

### Add
- `events` — `(id, aggregate_type, aggregate_id, version, type, payload JSONB, actor_id, created_at)` — append-only event store
- `outbox` — transactional outbox for reliable event delivery to River jobs
- `jobs` — River job queue table (managed by River)
- `hold_templates` — reusable hold configurations
- `approval_requests` — pending approval queue for destructive actions
- `breakglass_incidents` — post-incident review tracking
- `pbac_policies` — runtime context policies (or loaded from YAML, stored for reference)
- `cascade_state` — per-hold, per-custodian, per-provider state machine rows

### Modify
- `legal_holds` — add `template_id`, `expires_at` (enforcement changes), `status` enum
- `legal_hold_custodians` — add `cascade_status` enum
- `integration_instances` — add plugin identifier field, remove hardcoded type enum

### Keep (unchanged)
- `users`, `sessions`, `roles`, `user_roles`, `role_permissions`, `api_tokens`, `scim_groups`, `identity_cache`

### New: `vip_identities`
```sql
CREATE TABLE vip_identities (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email       TEXT NOT NULL UNIQUE,
  label       TEXT NOT NULL,            -- e.g. "C-Suite", "Board", "Legal Counsel"
  added_by    UUID REFERENCES users(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```
Used by the `vip_protection` PBAC policy. When an action targets an email present in this table, the policy returns `require_approval` regardless of the operator's role.

### On-Call Verification Interface

The `on_call_verification` PBAC policy needs to know if the acting operator is currently on-call. Rather than hardcoding a PagerDuty or OpsGenie client, the policy calls an `OnCallResolver` interface:

```go
type OnCallResolver interface {
    IsOnCall(ctx context.Context, email string) (bool, error)
}
```

Agent 2 implements the interface and a factory that reads `ON_CALL_PROVIDER` from config:
- `ON_CALL_PROVIDER=pagerduty` — uses PagerDuty REST API (`/oncalls` endpoint, filtered by user email)
- `ON_CALL_PROVIDER=opsgenie` — uses OpsGenie REST API (`/v2/schedules/on-calls`)
- `ON_CALL_PROVIDER=none` — returns `true` for all operators (disables the policy; safe default for orgs without on-call tooling)

Credentials (`ON_CALL_API_KEY`) are read from environment. The resolver is injected into `EvalContext` at request time.

---

## RBAC Permissions

All 26 canonical permissions. Wildcard matching applies: a role granted `holds:*` covers both `holds:read` and `holds:write`.

| Permission | Description |
|------------|-------------|
| `users:read` | View operator accounts |
| `users:write` | Create, update, deactivate operator accounts |
| `audit:read` | Query and export the event log |
| `identities:read` | Search and look up identities across integrations |
| `identities:write` | Refresh identity cache entries |
| `devices:read` | List and view device inventory |
| `integrations:read` | View available integrations and actions |
| `integrations:execute` | Execute actions against integrations |
| `instances:read` | View integration instance configurations |
| `instances:write` | Create, update, delete integration instances |
| `holds:read` | View legal holds and custodian lists |
| `holds:write` | Create, modify, and release legal holds |
| `hold_templates:read` | View hold templates |
| `hold_templates:write` | Create, update, delete hold templates |
| `roles:read` | View roles and permission assignments |
| `roles:write` | Create roles and assign permissions |
| `tokens:read` | View API tokens |
| `tokens:write` | Create and revoke API tokens |
| `approvals:read` | View the approval queue |
| `approvals:write` | Approve or reject pending actions |
| `breakglass:use` | Invoke the break-glass emergency override |
| `breakglass:review` | View break-glass incident list and post-incident records |
| `pbac_policies:read` | View PBAC policy configuration |
| `pbac_policies:write` | Create, update, delete PBAC policies |
| `vip_identities:read` | View the VIP identity list |
| `vip_identities:write` | Add or remove identities from the VIP list |
| `assistant:use` | Access the AI assistant |
| `scim:admin` | Manage SCIM configuration and group mappings |

**Built-in roles:**

| Role | Permissions |
|------|-------------|
| `admin` | All permissions |
| `operator` | `identities:*`, `devices:read`, `integrations:*`, `holds:*`, `hold_templates:read`, `approvals:*`, `breakglass:use`, `tokens:*`, `audit:read`, `assistant:use` |
| `auditor` | `audit:read`, `identities:read`, `holds:read`, `roles:read`, `tokens:read`, `devices:read`, `instances:read` |
| `legal_operator` | `holds:*`, `hold_templates:*`, `identities:read`, `audit:read` |
| `read_only` | `identities:read`, `devices:read`, `holds:read`, `audit:read`, `tokens:read` |
| `approver` | `approvals:*`, `breakglass:review`, `audit:read` |

---

## Agent Work Breakdown

The rebuild is divided into 7 agents working in sequence (with parallelism where noted). Each agent owns a vertical slice and produces a reviewable output.

---

### Agent 1: Foundation & Schema

**Owns:** Repository scaffolding, database schema, migration files, core types, and the event store.

**Deliverables:**
- `go.mod` with all dependencies: `connect-go`, `river`, `sqlc`, `golang-migrate`, `ory/fosite` (or `zitadel/oidc` for generic OIDC), `pgx/v5`
- All SQL migration files (full schema: events, outbox, jobs, cascade_state, hold_templates, approval_requests, breakglass_incidents, pbac_policies, plus all retained tables)
- sqlc config and generated query types for all tables
- Core domain types in `internal/domain/` (Hold, Custodian, CascadeState, Event, etc.)
- Event store service: `AppendEvent()`, `LoadEvents(aggregateID)`, `LoadEventsSince()`
- Transactional outbox pattern wired to River job insertion

**Must not:** Implement any API handlers, plugins, or frontend.

---

### Agent 2: Auth, RBAC, and PBAC

**Owns:** Generic OIDC SSO, session management, RBAC enforcement, PBAC policy engine, SCIM 2.0, break-glass procedure.

**Deliverables:**
- `internal/auth/` — OIDC flow with configurable issuer/client_id/client_secret/scopes (no Okta-specific code)
- Session creation, validation, and revocation
- `internal/rbac/` — full canonical permission set (see RBAC Permissions below), role assignment, wildcard permission matching
- `internal/pbac/` — YAML-defined policy engine implementing all 17 policies defined in the PBAC catalog (see delta §8). Policy interface: `Evaluate(ctx EvalContext, actor Actor, action Action) PolicyResult` where `PolicyResult` is `allow | deny | require_approval`. `EvalContext` carries: timestamp, source IP, geo-region, session age, session count, operator tenure, on-call roster membership, target identity flags (VIP, self), instance name, integration health, active incident flag, bulk action counters, active hold metadata. Each policy is a stateless function; the engine evaluates all matching policies and returns the most restrictive result. Ships with a default policy set covering `vip_protection`, `production_instance_gate`, `change_freeze_window`, and `step_up_mfa` with sane defaults.
- `internal/breakglass/` — break-glass request flow: reason capture, immediate execution, event emission, incident record creation, admin notification hook
- SCIM 2.0 handlers (Users + Groups endpoints, group-to-role mapping)
- Middleware: auth, RBAC check, PBAC check, rate limiting (internal + public)

**Depends on:** Agent 1 (event store for auth events, schema for sessions/users/roles)

---

### Agent 3: Plugin System & Core Integrations

**Owns:** Plugin interface definition and all 7 integration plugins (Okta, Google Workspace, Google Vault, Slack, Microsoft 365, Intune, JAMF).

**Plugin Interface:**
```go
type Plugin interface {
    ID() string
    RegisterActions() []ActionDefinition
    RegisterCredentialSchema() []CredentialField
    NewActionExecutor(creds Credentials) (ActionExecutor, error)
    HoldProvider(creds Credentials) (HoldProvider, error)   // nil if not supported
    IdentityProvider(creds Credentials) (IdentityProvider, error)
}
```

**Deliverables:**
- `internal/plugin/` — registry, loader, credential resolver (env var fallback → DB), action dispatcher
- `plugins/okta/` — identity lookup, deactivate, activate, set_blocked_access; no HoldProvider
- `plugins/google/` — identity lookup, suspend, activate, archive, reset_password, clear_sessions; HoldProvider via Google Vault
- `plugins/google_vault/` — matter management, MAIL/DRIVE/CHAT hold placement/removal
- `plugins/slack/` — identity lookup, clear_sessions, deactivate, reactivate; HoldProvider via policies
- `plugins/m365/` — litigation hold (Exchange), eDiscovery hold (SharePoint/OneDrive); HoldProvider
- `plugins/intune/` — device wipe, lock, retire; no HoldProvider
- `plugins/jamf/` — device lock, wipe (macOS + iOS); no HoldProvider
- Each plugin: credential schema, action definitions with `destructive: bool`, test coverage

**Depends on:** Agent 1 (domain types)

---

### Agent 4: Legal Hold Engine

**Owns:** Hold lifecycle, cascade state machine, River job workers, hold templates, auto-expiration, reconciliation.

**Deliverables:**
- `internal/legalhold/` — Hold service: create, add custodian, remove custodian, release, expire
- Cascade state machine: `CascadeStateMachine` managing per-custodian, per-provider state transitions (`pending → in_progress → completed | partial | failed`)
- River job workers:
  - `CascadePlaceJob` — places hold for one custodian on one provider; retries on failure; dead-letters after N attempts
  - `CascadeRemoveJob` — removes hold for one custodian on one provider (only if no other active holds)
  - `ReconcileHoldsJob` — scheduled every 2 minutes; per-provider drift detection; emits `hold.drift_detected` events
  - `ExpireHoldJob` — scheduled per hold; releases hold and cascades removal when `expires_at` passes
- Hold templates: CRUD, glob-matched provider targeting, default expiration, notes templates
- All state transitions emit events to the event store

**Depends on:** Agents 1, 3

---

### Agent 5: API Layer (connect-go)

**Owns:** All protobuf definitions, connect-go server implementation, two distinct API surfaces (internal and public), approval workflow handlers.

**Two API Surfaces**

Warden exposes two fully separate API surfaces mounted at different path prefixes with different authentication mechanisms, different middleware stacks, and different rate limit profiles. The same underlying service implementations handle both — the difference is entirely in the transport/auth layer.

**Internal API** (`/api/v1/internal/*`)
- **Auth:** Session cookie set at login via the OIDC callback. Every request validates the session token against the `sessions` table. Expired or missing session → 401 redirect to login.
- **Consumer:** The Vue frontend exclusively. Not intended for scripts or external systems.
- **Rate limiting:** Lenient (trusts authenticated operators behind SSO). Burst allowed.
- **Endpoints:**
  - `POST /auth/login`, `GET /auth/callback`, `POST /auth/logout` — OIDC flow
  - `GET /me` — current user profile + effective permissions
  - `POST /identities/search`, `GET /identities/:email` — live identity lookup across instances
  - `POST /actions/:instance/:action` — execute action (RBAC + PBAC gate; routes to approval queue if destructive)
  - `GET /holds`, `POST /holds`, `GET /holds/:id`, `DELETE /holds/:id/release` — hold lifecycle
  - `POST /holds/:id/custodians`, `DELETE /holds/:id/custodians/:email` — custodian management
  - `GET /holds/:id/cascade-status` — per-custodian, per-provider cascade state breakdown
  - `GET /audit` — event log query (filterable by aggregate, actor, type, date range, paginated)
  - `GET /audit/export` — export as JSON or CSV (streams response)
  - `GET /approvals`, `POST /approvals/:id/approve`, `POST /approvals/:id/reject` — approval queue
  - `POST /breakglass` — emergency override with reason capture
  - `GET /breakglass/incidents` — post-incident review list (admin only)
  - `GET /tokens`, `POST /tokens`, `DELETE /tokens/:id` — personal API token management
  - `GET /admin/instances`, `POST /admin/instances`, `PUT /admin/instances/:id`, `DELETE /admin/instances/:id`
  - `GET /admin/roles`, `POST /admin/roles`, `PUT /admin/roles/:id/permissions`
  - `GET /admin/scim-mappings`, `PUT /admin/scim-mappings`
  - `GET /admin/hold-templates`, `POST /admin/hold-templates`, `PUT /admin/hold-templates/:id`, `DELETE /admin/hold-templates/:id`
  - `GET /admin/pbac-policies`, `PUT /admin/pbac-policies`
  - `POST /assistant/chat` — Claude tool-use SSE stream

**Public API** (`/api/v1/public/*`)
- **Auth:** Bearer token (`Authorization: Bearer <token>`). Tokens are SHA-256 hashed in the `api_tokens` table. Each token has a `scopes` field — a list of permitted action keys (e.g., `okta-prod:deactivate_user`, `holds:write`, `audit:read`). A request is rejected if the token's scopes do not cover the requested operation, even if the token is valid.
- **Consumer:** Automated scripts, SOAR playbooks, CI/CD pipelines, external tooling. Designed for machine-to-machine use.
- **Rate limiting:** Strict per-token rate limiting. Configurable per token at creation time (default: 60 req/min). Exceeding the limit → 429 with `Retry-After` header.
- **Scope model:** Scopes are dot-notation strings matching `resource.action` or `instance.resource.action`. A token with scope `holds.*` can read and write holds but cannot execute actions. A token with scope `okta-prod.users.deactivate` can only deactivate users on that specific instance. Wildcard `*` is reserved for admin-issued tokens only.
- **Endpoints (subset of internal, no admin or assistant):**
  - `POST /actions/:instance/:action` — execute action (token scope checked first, then RBAC + PBAC)
  - `GET /holds`, `POST /holds`, `GET /holds/:id`, `DELETE /holds/:id/release`
  - `POST /holds/:id/custodians`, `DELETE /holds/:id/custodians/:email`
  - `GET /audit` — query audit events (read-only)
  - `GET /identities/search` — identity lookup (read-only)

**Shared Behavior (both surfaces)**
- All action executions — regardless of surface — go through the same RBAC + PBAC middleware and emit to the same event store. The audit trail does not distinguish internal vs. public calls by surface; it records `actor_type: user | token` and the actor's ID.
- SCIM 2.0 is a third surface (`/scim/v2/*`) with its own bearer token (separate from public API tokens) and is handled by Agent 2.

**Deliverables:**
- `proto/` — protobuf definitions for all services: IdentityService, ActionService, HoldService, AuditService, ApprovalService, AdminService, AssistantService
- Generated connect-go stubs
- `internal/api/internal/` — session-auth middleware + all internal route handlers
- `internal/api/public/` — token-auth middleware + token scope enforcement + all public route handlers
- `internal/api/middleware/` — shared: RBAC check, PBAC check, request logging, security headers, Prometheus metrics; surface-specific: session validation, bearer token validation, rate limiters
- `internal/api/router.go` — mounts both surfaces under their path prefixes with their respective middleware chains
- OpenAPI docs auto-generated from proto (one spec per surface)

**Depends on:** Agents 1, 2, 3, 4

---

### Agent 6: Frontend

**Owns:** Vue 3 frontend — all views, components, mobile-first layout, dark/light theme, connect-web integration.

**Deliverables:**
- Project scaffold: Vite, Vue 3, TypeScript, TailwindCSS v4, Pinia, Vue Router, `@connectrpc/connect-web`
- Layout: responsive shell with bottom nav (mobile), sidebar nav (desktop), theme toggle
- `views/LoginView` — OIDC redirect flow
- `views/DashboardView` — stats, live audit stream, pending approval count badge
- `views/IdentitiesView` — search, per-instance result cards, action panel (action sheets on mobile), approval request confirmation for destructive actions
- `views/DevicesView` — JAMF device listing, lock/wipe with approval gate
- `views/AuditView` — event log query (filterable by aggregate, actor, type, date range), export
- `views/LegalHoldView` — hold list, create from template or scratch, custodian management, cascade status breakdown per provider, hold template management
- `views/ApprovalView` — pending approval queue for approvers; approve/reject with reason
- `views/BreakGlassView` — emergency override form; incident list for admins
- `views/SettingsView` — roles, permissions, instances, SCIM mappings, PBAC policies, hold templates, config export/import
- Dark + light theme: `prefers-color-scheme` detection on first visit, toggle persisted in localStorage
- Permission-based route guards and UI element visibility (keyed to the canonical RBAC permission set)
- Mobile-first: 60-second critical flow (alert → search → suspend) fully functional on phone

**Depends on:** Agent 5 (proto/connect types)

---

### Agent 7: Infrastructure, CI/CD, and Configuration-as-Code

**Owns:** Containerized development environment, Docker, Helm chart, GitHub Actions, YAML config schema, declarative apply tooling.

**Development Environment Constraint:** Warden is developed entirely inside containers. No Go, Node, sqlc, buf, or any other toolchain is installed on the host machine. The only host dependencies are Docker Desktop and VS Code (or any editor with Dev Container support). All compilation, codegen, testing, linting, and running happens inside containers. This applies on Windows, macOS, and Linux — the experience is identical on all three.

**Deliverables:**

*Dev Container*
- `.devcontainer/devcontainer.json` — VS Code Dev Container definition. Base image: `mcr.microsoft.com/devcontainers/go` (Debian-based, Go pre-installed). Additional features: Node.js (for frontend), Docker-in-Docker (for building images from inside the container), GitHub CLI.
- `.devcontainer/Dockerfile` — installs all toolchain dependencies on top of the base image: `sqlc`, `buf` (protobuf codegen), `golangci-lint`, `air` (Go hot-reload), `golang-migrate` CLI, `warden-ctl` (built from source on container start).
- `.devcontainer/docker-compose.yml` — the dev container itself plus all services it depends on: PostgreSQL 16, River UI dashboard (for job queue visibility), a mock OIDC provider (for local SSO without a real IdP). The backend and frontend run inside the dev container itself (not as separate services) so the developer can attach a debugger.
- `Makefile` — all developer commands run via `make` inside the container. No raw toolchain invocations required:
  - `make dev` — starts backend (air hot-reload) + frontend (vite dev server) concurrently
  - `make migrate` — runs pending migrations against the local Postgres
  - `make migrate-create name=<name>` — scaffolds a new migration file
  - `make generate` — runs sqlc + buf codegen
  - `make test` — runs all Go tests with `-race`
  - `make lint` — runs golangci-lint + vue-tsc + eslint
  - `make build` — produces a production binary + frontend bundle
  - `make ctl-apply dir=<path>` — runs warden-ctl apply against local instance

*Production Containers*
- `Dockerfile` — multi-stage: stage 1 builds Go binary (linux/amd64, CGO_ENABLED=0), stage 2 builds Vue bundle, stage 3 is distroless final image containing only the binary and static assets. No shell, no package manager in the final image.
- `docker-compose.prod.yml` — single-node production reference: Warden, Postgres, Caddy (TLS termination + reverse proxy). Not the recommended production path (use Helm) but useful for non-Kubernetes deployments.

*Kubernetes / Helm*
- `charts/warden/` — Helm chart: Deployment, Service, Ingress, ConfigMap, Secret, HPA, PodDisruptionBudget, ServiceMonitor (Prometheus). Values file has opinionated defaults with comments for all tunable fields.

*CI/CD*
- `.github/workflows/ci.yml` — runs entirely in containers (uses `docker buildx`): lint (golangci-lint, vue-tsc, eslint), unit tests, integration tests (spins up Postgres via service container), build image, helm lint
- `.github/workflows/release.yml` — on tag push: build multi-arch image (linux/amd64, linux/arm64), push to registry, package Helm chart, create GitHub Release

*Configuration-as-Code*
- `config/schema/` — JSON Schema for all YAML config files: roles, permissions, pbac_policies, scim_mappings, integration_instances
- `cmd/warden-ctl/` — CLI tool (also runs inside the dev container): `warden-ctl apply -f config/`, `warden-ctl export`, `warden-ctl diff`
- `docs/` — architecture diagram, API reference (auto-generated from proto), deployment guide, runbook, plugin authoring guide, dev container setup guide (one-page: install Docker Desktop, clone repo, open in VS Code, click "Reopen in Container")

**Depends on:** Agents 1–6 (integrates final outputs)

---

## Development Environment

Warden is developed entirely inside containers. **Nothing is installed on the host machine except Docker Desktop and an editor.** This is a hard constraint — it ensures the development environment is identical regardless of whether the host is Windows, macOS, or Linux, and eliminates all "works on my machine" issues caused by toolchain version drift.

**Host requirements (only these, nothing else):**
- Docker Desktop (Windows: WSL2 backend required)
- VS Code with the Dev Containers extension (or any editor with devcontainer support)
- Git

**What runs on the host:** Nothing. Git clone and open in container.

**What runs inside the container:**
- Go compiler and toolchain
- Node.js and npm (frontend)
- `sqlc` — type-safe query codegen from SQL
- `buf` — protobuf/connect-go codegen
- `air` — Go hot-reload for backend development
- `golangci-lint`, `vue-tsc`, `eslint` — linters
- `golang-migrate` CLI — migration management
- `warden-ctl` — built from source on container start
- All `make` targets

**Local service topology (docker-compose inside dev container):**

```
┌─────────────────────────────────────────────────────┐
│                 Dev Container                       │
│  ┌────────────────────┐  ┌────────────────────┐    │
│  │  Backend (air)     │  │  Frontend (vite)   │    │
│  │  :8080             │  │  :5173             │    │
│  └────────────────────┘  └────────────────────┘    │
└─────────────────────────────────────────────────────┘
         │                          │
┌────────▼──────────┐   ┌───────────▼──────────────┐
│  PostgreSQL 16    │   │  Mock OIDC Provider       │
│  :5432            │   │  (no real IdP needed      │
│                   │   │   for local dev)          │
└───────────────────┘   └──────────────────────────┘
         │
┌────────▼──────────┐
│  River UI         │
│  :3000            │
│  (job queue       │
│   dashboard)      │
└───────────────────┘
```

**Developer workflow:**
1. `git clone` the repo on the host
2. Open in VS Code → "Reopen in Container" (one click)
3. Container builds (~2 min first time, cached after)
4. `make dev` — backend hot-reloads on Go file changes, frontend HMR on Vue file changes
5. All other operations via `make` targets — no raw toolchain commands needed

**No Windows-specific code, paths, or assumptions exist anywhere in the codebase.** The final production image is `linux/amd64` (and `linux/arm64`). The dev container is Debian-based. CI runs on Linux runners. The Windows host is purely a Docker Desktop host — it contributes nothing to the build.

---

## Build Sequence

```
Phase 1 (no dependencies — can start immediately)
  └─ Agent 1: Foundation & Schema

Phase 2 (depends on Agent 1 — can run in parallel with each other)
  ├─ Agent 2: Auth, RBAC, PBAC
  └─ Agent 3: Plugin System & Integrations

Phase 3 (depends on Agents 1, 2, 3)
  └─ Agent 4: Legal Hold Engine

Phase 4 (depends on Agents 1–4)
  └─ Agent 5: API Layer

Phase 5 (depends on Agent 5)
  └─ Agent 6: Frontend

Phase 6 (depends on Agents 1–6)
  └─ Agent 7: Infrastructure & Config-as-Code
```

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Query layer | sqlc | Type-safe, no ORM magic, schema is the source of truth |
| Job queue | River (PostgreSQL-backed) | No new infrastructure; jobs survive restarts; goroutine-based approaches lose work on restart |
| API protocol | connect-go (gRPC + REST) | One proto definition, two protocols; internal efficiency + external compatibility |
| OIDC | `zitadel/oidc` | Provider-agnostic; no IdP-specific SDK |
| Mock OIDC (dev) | Dex | Lightweight, runs in docker-compose, no real IdP needed locally |
| Go module path | `github.com/aechrok/warden` | Matches the GitHub repo |
| Event store | Append-only events table (PostgreSQL) | No Kafka overhead; fits the scale; replay and time-travel built in |
| Plugin loading | `init()` registration | No dynamic loading; compile-time safety; clear import graph |
| PBAC policies | YAML files + DB storage | GitOps-friendly; UI-editable; declarative |
| Frontend protocol | connect-web | Type-safe from proto; same definitions as backend |
| Mobile layout | Bottom nav + action sheets | Standard mobile patterns; 60-second critical flow |
| Theme | `prefers-color-scheme` + toggle | Accessibility; enterprise requirement |

---

## What Warden Is Not

- A SIEM — Warden does not ingest logs from external systems; it emits its own event stream
- An MDM — Warden delegates device commands to JAMF/Intune; it does not manage devices natively
- A SOAR — Warden has no automated playbook engine; approvals and break-glass are human-in-the-loop
- Multi-tenant SaaS — Warden is a single-tenant, self-hosted deployment; one org per instance

---

## Success Criteria

- [ ] All 7 integration plugins ship: Okta, Google Workspace, Google Vault, Slack, M365, Intune, JAMF
- [ ] Legal hold cascade is durable: server restart mid-cascade resumes from last successful step
- [ ] Cascade state machine is visible in the UI per hold, per custodian, per provider
- [ ] A new OIDC provider can be configured without code changes
- [ ] Approval workflow gates all actions marked `destructive: true`
- [ ] Break-glass creates an auditable incident and notifies admins
- [ ] PBAC policy blocks destructive actions outside business hours (demo policy ships by default)
- [ ] Hold templates allow one-click creation of a standard litigation hold
- [ ] All config (roles, policies, instances) can be expressed as YAML and applied via `warden-ctl apply`
- [ ] Critical flow (alert → search → suspend) completes on mobile in under 60 seconds
- [ ] Dark and light themes both pass WCAG AA contrast
- [ ] Zero Okta-specific code anywhere outside `plugins/okta/`
