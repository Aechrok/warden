# Warden — Claude Handoff

## Project Overview

Warden is an identity control-plane: it sits in front of integrations (Okta, Google, Slack, JAMF, Intune, M365, Google Vault) and provides centralized identity lookup, lifecycle actions, legal holds, break-glass access, RBAC/PBAC policy enforcement, and audit logging.

**Stack:** Go backend (stdlib HTTP, pgx/v5, River job queue) · Vue 3 + TypeScript frontend · PostgreSQL · Docker Compose local dev

**Key constraint: everything runs in containers. Never run npm/go/node on the host.**

---

## Running the Stack

```bash
# Build and start everything
docker compose -f docker-compose.local.yml up -d

# Rebuild warden only (Go + frontend)
docker compose -f docker-compose.local.yml build warden
docker compose -f docker-compose.local.yml up -d --force-recreate warden

# When TypeScript errors block Docker layer cache, force a clean build
docker compose -f docker-compose.local.yml build --no-cache warden

# Tail logs
docker compose -f docker-compose.local.yml logs warden -f

# Restart frontend dev server (required when new .vue files are added — HMR doesn't detect new files on Docker Desktop)
docker compose -f docker-compose.local.yml restart frontend
```

---

## Architecture

### Backend (`internal/`)

- `cmd/server/main.go` — entrypoint; blank-imports all plugins via `_ "github.com/aechrok/warden/plugins/okta"` etc. so their `init()` functions fire and register them into the global plugin registry
- `internal/api/server.go` — wires all services; creates the HTTP server
- `internal/api/router.go` — mounts all routes
- `internal/api/internal/handlers.go` — all session-auth internal API handlers (~1500 lines)
- `internal/api/public/handlers.go` — bearer-token public API handlers
- `internal/plugin/registry.go` — global plugin registry; plugins call `plugin.Register(New())` in their `init()`
- `internal/plugin/dispatcher.go` — routes GetIdentity/Execute/PlaceHold calls to the right plugin; `NewDispatcher(nil, resolver, pool)` uses the global registry
- `internal/plugin/resolver.go` — decrypts credentials from `integration_instances.credentials_enc`; overlays env var overrides (`{INSTANCE_NAME_UPPER}_{FIELD_KEY_UPPER}`)
- `internal/domain/plugin.go` — core interfaces: `Plugin`, `ActionExecutor`, `IdentityProvider`, `HoldProvider`, `ActionDefinition`

### Plugin System

Plugins live in `plugins/{name}/plugin.go`. Each calls `plugin.Register(New())` in `init()`. The global registry is populated before `api.NewServer` runs.

**Critical:** `plugin.NewRegistry()` creates an EMPTY isolated registry (tests only). Always use `plugin.GlobalRegistry()` or pass `nil` to `NewDispatcher` for production code.

### Frontend (`frontend/src/`)

- `views/IdentitiesView.vue` — identity search, tab-per-instance layout, action panel
- `views/DevicesView.vue` — JAMF device search, lock/wipe actions
- `components/IdentityActionPanel.vue` — action panel with state-aware buttons (destructive=red, hold-blocked=purple/disabled)
- `components/DebugPanel.vue` — API calls, DB queries, loaded plugins tabs
- `api/identities.ts` — returns `IdentitySearchResponse { identities, on_hold }`
- `api/actions.ts` — `listActions()` returns flat `ActionDef[]` with `instance_id` on each

---

## Key Data Shapes

### `GET /api/v1/internal/identities/search?email=&instance_id=`
```json
{
  "identities": [{ "email", "display_name", "instance_id", "instance_name", "data", "fetched_at" }],
  "on_hold": false
}
```
- `instance_id` optional; omit to fan out across all active instances
- Fan-out skips instances that error (logged at Warn level)

### `GET /api/v1/internal/actions/`
```json
{
  "actions": [{
    "key", "label", "description",
    "instance_id", "plugin",
    "destructive", "requires_approval",
    "applicable_states": ["ACTIVE", "SUSPENDED"]
  }]
}
```
- `applicable_states` — if non-empty, action is only shown when the identity's `data.status` matches; empty means always show
- Frontend filters in `actionsForInstance(identity)` in `IdentitiesView.vue`

### `GET /api/v1/internal/admin/plugins?loaded_only=true`
- Without `?loaded_only=true` → returns all registered plugins (used by Settings → Instances dropdown)
- With `?loaded_only=true` → returns only plugins with ≥1 active instance (used by Debug panel)
- Each plugin has `{ id, name, schema, loaded }` where `loaded` indicates active instance exists

---

## Okta Plugin (`plugins/okta/plugin.go`)

Actions with their applicable states:
- `reset_password` → ACTIVE, RECOVERY, LOCKED_OUT, PASSWORD_EXPIRED
- `reset_factors` → ACTIVE, RECOVERY, LOCKED_OUT
- `clear_sessions` → ACTIVE, RECOVERY, LOCKED_OUT, PASSWORD_EXPIRED, SUSPENDED
- `suspend_user` → ACTIVE, RECOVERY, LOCKED_OUT, PASSWORD_EXPIRED
- `unsuspend_user` → SUSPENDED
- `deactivate_user` → ACTIVE, SUSPENDED, RECOVERY, LOCKED_OUT, PASSWORD_EXPIRED, PROVISIONED
- `activate_user` (reactivate) → DEPROVISIONED, PROVISIONED

Credential fields: `base_url` (e.g. `https://your-org.okta.com`), `api_token`

Identity lookup: `GET /api/v1/users/{email}` — returns full Okta user object including `status`

---

## Bugs Fixed This Session (important context)

### Credential merge on instance update
`UpdateInstance` now decrypts existing credentials and overlays only non-blank submitted fields. Previously it replaced the entire credentials object, silently wiping fields the UI left blank (secret fields render blank on the edit form since we can't show stored secrets).

### Plugin registry was empty
`server.go` was calling `plugin.NewRegistry()` (empty, test-only) instead of using the global. Fixed by passing `nil` to `NewDispatcher` which falls back to `global`.

### `ListActions` response shape mismatch
Backend returned `{ "instances": [...] }` but frontend expected `{ "actions": [...] }`. Frontend was always getting `undefined` → empty array. Fixed by flattening to `{ "actions": [...] }` with `instance_id` on each action.

### Error messages from upstream
Identity search for a specific instance used to return vague "lookup failed" 500 for all upstream errors. Now returns:
- 401 upstream → 401 + "check integration credentials"
- 403 upstream → 403 + "token lacks required permissions"
- 404 upstream → 404 + "identity not found"
- Other upstream → 502 + status code
Uses `UpstreamHTTPStatus()` method on `httpx.APIError` + a local interface in handlers.go (can't import `plugins/internal/httpx` from `internal/api/internal` due to Go's internal package rules).

### Fan-out errors were silent
All-instances fan-out logged errors at `Debug` level (invisible in production logger). Changed to `Warn`.

---

## Patterns to Follow

- **Error from upstream plugin:** check `upstreamStatus(err)` / `upstreamMessage(err)` helpers in `handlers.go`
- **New action on a plugin:** add to `Actions()` with `ApplicableStates` set; no other changes needed for it to appear in the UI
- **New plugin:** create `plugins/{name}/plugin.go`, call `plugin.Register(New())` in `init()`, blank-import in `cmd/server/main.go`
- **Credential field:** add to `CredentialSchema()` — the Settings UI renders it dynamically; secret fields render as password inputs

## Things NOT to do

- Don't use `plugin.NewRegistry()` in production code — it's empty
- Don't run npm/go/node directly — always use docker compose
- Don't commit with Co-Authored-By lines
- Don't use `NewDispatcher(plugin.NewRegistry(), ...)` — pass `nil` for global
