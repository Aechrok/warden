# Warden — Architecture

## Component Overview

```
                          ┌─────────────────────────────────────────┐
                          │               Browser / CLI              │
                          │   Vue 3 SPA (Vite)    warden-ctl        │
                          └───────────┬─────────────────┬───────────┘
                                      │ HTTPS            │ HTTPS
                          ┌───────────▼─────────────────▼───────────┐
                          │             Warden Server                │
                          │  ┌──────────────────────────────────┐   │
                          │  │         HTTP ServeMux            │   │
                          │  │  /auth/    /scim/v2/             │   │
                          │  │  /api/v1/internal/               │   │
                          │  │  /api/v1/public/                 │   │
                          │  └───┬──────────┬──────────┬────────┘   │
                          │      │          │          │            │
                          │  ┌───▼──┐  ┌───▼──┐  ┌───▼──────┐    │
                          │  │ RBAC │  │ PBAC │  │  Plugin   │    │
                          │  │Check │  │Engine│  │ Registry  │    │
                          │  └───┬──┘  └───┬──┘  └───┬──────┘    │
                          │      └──────────┴──────────┘          │
                          │                  │                     │
                          │  ┌───────────────▼───────────────┐    │
                          │  │         PostgreSQL 16          │    │
                          │  │  users / sessions / events     │    │
                          │  │  holds / cascade_state         │    │
                          │  │  pbac_policies / vip_ids       │    │
                          │  │  River job queue (outbox)      │    │
                          │  └───────────────────────────────┘    │
                          │                                        │
                          │  ┌───────────────────────────────┐    │
                          │  │        River Workers          │    │
                          │  │  CascadePlaceJob              │    │
                          │  │  CascadeRemoveJob             │    │
                          │  │  ReconcileHoldsJob (2 min)    │    │
                          │  │  ExpireHoldJob                │    │
                          │  └───────────────────────────────┘    │
                          └────────────────────────────────────────┘
                                           │
                   ┌───────────────────────┼───────────────────────┐
                   │                       │                       │
          ┌────────▼───────┐    ┌──────────▼──────┐    ┌──────────▼──────┐
          │   Okta / OIDC  │    │  Google Workspace│    │  Slack / M365   │
          │   Intune / JAMF│    │  Google Vault    │    │  (etc.)         │
          └────────────────┘    └─────────────────┘    └─────────────────┘
```

## Data Flow — Action Execution

```
Browser                  Warden Server               Plugin          Event Store
  │                           │                         │                │
  │  POST /actions/execute     │                         │                │
  │  {plugin, action, target}  │                         │                │
  ├──────────────────────────►│                         │                │
  │                           │ 1. Session auth check   │                │
  │                           │ 2. RBAC: integrations:exec              │
  │                           │ 3. PBAC: eval all enabled policies      │
  │                           │    (freeze window, vip, on-call, etc.)  │
  │                           │                         │                │
  │                           │ 4. If PBAC blocks →     │                │
  │  ◄── 403 (policy detail) ─┤    return early         │                │
  │                           │                         │                │
  │                           │ 5. Dispatch to plugin   │                │
  │                           ├────────────────────────►│                │
  │                           │                         │ 6. HTTP call   │
  │                           │                         │    to external │
  │                           │                         │    provider    │
  │                           │◄────────────────────────┤                │
  │                           │                         │                │
  │                           │ 7. AppendEvent(action.executed)         │
  │                           ├────────────────────────────────────────►│
  │                           │                         │                │
  │  ◄── 200 {result}  ───────┤                         │                │
```

## Event Sourcing Model

All state-changing operations append an immutable event to the `events` table before (or as part of) the state mutation. This provides:

- **Complete audit trail**: every action, hold change, break-glass invocation, and policy evaluation result is recorded.
- **Replayability**: the event log can be replayed to reconstruct state or feed external SIEM systems.
- **Optimistic concurrency**: the event store tracks `version` per aggregate; conflicting writes are rejected.

### Event table schema

```sql
CREATE TABLE events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate   TEXT NOT NULL,   -- e.g. "hold", "user", "breakglass"
    agg_id      UUID NOT NULL,
    type        TEXT NOT NULL,   -- e.g. "hold.placed", "action.executed"
    actor_id    UUID,
    actor_email TEXT,
    payload     JSONB NOT NULL,
    version     BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Transactional Outbox

State mutations that need async side effects (River job enqueue, webhook delivery) write to an `outbox` table in the same DB transaction. A River worker drains the outbox and delivers messages, guaranteeing at-least-once delivery without distributed transactions.

## Security Layers

1. **Session auth** (internal API): cookie-backed session validated on every request.
2. **Token auth** (public API + SCIM): bearer token with per-token scopes stored in `api_tokens`.
3. **RBAC**: 26 permissions mapped to 6 built-in roles; wildcard matching (`*`) for superadmin.
4. **PBAC**: up to 17 configurable policies evaluated after RBAC; any DENY stops the request.
5. **Encryption**: credentials at rest encrypted with AES-256-GCM using `ENCRYPTION_KEY`.
