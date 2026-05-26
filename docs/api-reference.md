# Warden — API Reference

All responses are `application/json`. Error bodies: `{"error": "<message>"}`.

## Authentication

| Surface | Method | Header / Cookie |
|---------|--------|-----------------|
| Internal API | Session cookie | `Set-Cookie: session=<token>` after login |
| Public API | Bearer token | `Authorization: Bearer <api-token>` |
| SCIM 2.0 | Bearer token | `Authorization: Bearer <api-token>` (scope: `scim:admin`) |

---

## Auth Endpoints (no auth required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/auth/login` | Redirects to OIDC provider |
| GET | `/auth/callback` | OIDC callback; sets session cookie; redirects to `/` |
| POST | `/api/v1/internal/auth/logout` | Revokes session |

---

## Internal API — `/api/v1/internal/`

Session cookie required on all routes.

### Me

| Method | Path | Permission | Response |
|--------|------|------------|----------|
| GET | `/api/v1/internal/me` | (any session) | `{id, email, name, roles[], permissions[]}` |

### Identities

| Method | Path | Permission | Body / Query | Response |
|--------|------|------------|--------------|----------|
| GET | `/api/v1/internal/identities/search` | `identities:read` | `?q=<term>&instance=<id>` | `{results: [{id, email, name, instance_id, ...}]}` |
| POST | `/api/v1/internal/identities/cache/refresh` | `identities:write` | `{instance_id}` | `204` |

### Actions

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| POST | `/api/v1/internal/actions/execute` | `integrations:exec` + PBAC | `{plugin_id, action, target_id, instance_id, reason?}` | `{result, event_id}` |
| GET | `/api/v1/internal/actions/` | `integrations:read` | — | `{actions: [{plugin_id, action, label, destructive}]}` |

### Holds

| Method | Path | Permission | Body / Param | Response |
|--------|------|------------|--------------|----------|
| GET | `/api/v1/internal/holds/` | `holds:read` | — | `{holds: [Hold]}` |
| POST | `/api/v1/internal/holds/` | `holds:write` | `{name, template_id?, custodian_emails[], expiration_days?}` | `Hold` |
| GET | `/api/v1/internal/holds/{id}` | `holds:read` | — | `Hold` with cascade states |
| POST | `/api/v1/internal/holds/{id}/custodians` | `holds:write` | `{email}` | `204` |
| DELETE | `/api/v1/internal/holds/{id}/custodians/{custodianId}` | `holds:write` | — | `204` |
| POST | `/api/v1/internal/holds/{id}/release` | `holds:write` | `{reason}` | `204` |

**Hold object**: `{id, name, status, custodians[], created_at, expires_at, cascade_state}`

### Audit

| Method | Path | Permission | Query | Response |
|--------|------|------------|-------|----------|
| GET | `/api/v1/internal/audit/events` | `audit:read` | `?since=<iso>&limit=<n>&actor=<email>&type=<type>` | `{events: [Event]}` |
| GET | `/api/v1/internal/audit/export` | `audit:read` | `?since=<iso>&format=csv\|json` | `text/csv` or `application/json` |

**Event object**: `{id, aggregate, agg_id, type, actor_id, actor_email, payload, version, created_at}`

### Approvals

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/approvals/` | `approvals:read` | — | `{approvals: [Approval]}` |
| POST | `/api/v1/internal/approvals/{id}/approve` | `approvals:write` | `{note?}` | `204` |
| POST | `/api/v1/internal/approvals/{id}/reject` | `approvals:write` | `{note?}` | `204` |

### Break-Glass

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| POST | `/api/v1/internal/breakglass/invoke` | `breakglass:use` | `{reason (≥20 chars), plugin_id, action, target_id, instance_id}` | `{incident_id, result}` |
| GET | `/api/v1/internal/breakglass/incidents` | `breakglass:review` | — | `{incidents: [Incident]}` |
| POST | `/api/v1/internal/breakglass/incidents/{id}/review` | `breakglass:review` | `{note}` | `204` |

### API Tokens

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/tokens` | `tokens:read` | — | `{tokens: [Token]}` |
| POST | `/api/v1/internal/tokens` | `tokens:read` | `{name, scopes[], expires_at?}` | `{token: "<raw>", meta: Token}` |
| DELETE | `/api/v1/internal/tokens/{id}` | `tokens:write` | — | `204` |

### Admin — Instances

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/admin/instances` | `instances:read` | — | `{instances: [Instance]}` |
| POST | `/api/v1/internal/admin/instances` | `instances:read` | `{name, plugin_id, credentials{}}` | `Instance` |
| PUT | `/api/v1/internal/admin/instances/{name}` | `instances:write` | `{credentials{}}` | `Instance` |
| DELETE | `/api/v1/internal/admin/instances/{name}` | `instances:write` | — | `204` |

### Admin — Roles

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/admin/roles` | `roles:read` | — | `{roles: [Role]}` |
| POST | `/api/v1/internal/admin/roles/{name}/assign` | `roles:write` | `{user_id}` | `204` |
| DELETE | `/api/v1/internal/admin/roles/{name}/users/{userId}` | `roles:write` | — | `204` |

### Admin — PBAC Policies

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/admin/pbac` | `pbac:read` | — | `{policies: [Policy]}` |
| PUT | `/api/v1/internal/admin/pbac/{name}` | `pbac:write` | `{enabled, config{}}` | `Policy` |

### Admin — Hold Templates

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/admin/hold-templates` | `hold_templates:read` | — | `{templates: [Template]}` |
| POST | `/api/v1/internal/admin/hold-templates` | `hold_templates:read` | `{name, description, provider_glob, expiration_days}` | `Template` |
| PUT | `/api/v1/internal/admin/hold-templates/{name}` | `hold_templates:write` | same as POST | `Template` |
| DELETE | `/api/v1/internal/admin/hold-templates/{name}` | `hold_templates:write` | — | `204` |

### Admin — VIP Identities

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| GET | `/api/v1/internal/admin/vip` | `vip:read` | — | `{identities: [VIP]}` |
| POST | `/api/v1/internal/admin/vip` | `vip:read` | `{email, reason}` | `VIP` |
| DELETE | `/api/v1/internal/admin/vip/{email}` | `vip:write` | — | `204` |

### Assistant

| Method | Path | Permission | Body | Response |
|--------|------|------------|------|----------|
| POST | `/api/v1/internal/assistant/stream` | `assistant:use` | `{message}` | `text/event-stream` SSE |

---

## Public API — `/api/v1/public/`

Bearer token required. Token must have the appropriate scope.

| Method | Path | Required Scope | Description |
|--------|------|----------------|-------------|
| POST | `/api/v1/public/actions/execute` | `actions:execute` | Execute a plugin action |
| GET | `/api/v1/public/holds` | `holds:read` | List holds |
| POST | `/api/v1/public/holds` | `holds:write` | Create a hold |
| GET | `/api/v1/public/audit/events` | `audit:read` | Query audit log |
| GET | `/api/v1/public/identities/search` | `identities:read` | Search identities |

---

## SCIM 2.0 — `/scim/v2/`

Bearer token with scope `scim:admin`. Implements RFC 7644.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/scim/v2/Users` | List users |
| GET | `/scim/v2/Users/{id}` | Get user |
| POST | `/scim/v2/Users` | Provision user |
| PUT | `/scim/v2/Users/{id}` | Replace user |
| DELETE | `/scim/v2/Users/{id}` | Deprovision user |
| GET | `/scim/v2/Groups` | List groups |
| GET | `/scim/v2/Groups/{id}` | Get group |
| POST | `/scim/v2/Groups` | Create group |
| PUT | `/scim/v2/Groups/{id}` | Replace group (syncs role mapping) |
| DELETE | `/scim/v2/Groups/{id}` | Delete group |
