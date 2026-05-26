# Warden

Warden is a centralized IT security control plane. Operators use it to look
up identities across configured integrations (Okta, Google Workspace,
Slack, JAMF, Microsoft 365, Intune), execute high-impact actions
(suspend, wipe, deactivate), and place legal holds that cascade to
downstream systems. Every operation is recorded in an append-only event
store; durable cascades and outbox dispatch run as River jobs.

## Status

Foundation only. The repository currently contains:

- Go module `github.com/aechrok/warden`
- SQL migrations for identity, RBAC, integrations, holds, cascade state,
  outbox, events, approvals, break-glass, PBAC, and VIP records
- `sqlc.yaml` and query files for the foundation tables
- Domain types and plugin interfaces under `internal/domain`
- Transactional event store and outbox under `internal/store`
- Env-driven config loader under `internal/config`
- A `cmd/server/main.go` stub that loads config, opens the pgx pool,
  applies migrations, and blocks on signal

No HTTP server, no plugins, no frontend yet. See `TODO.md` and
`WARDEN_PLAN.md` for the full build plan.

## Layout

```
cmd/server/         server entry point
internal/config/    env-only configuration loader
internal/domain/    core domain types and plugin interfaces
internal/db/        migrations (golang-migrate) and sqlc query files
internal/store/     event store and transactional outbox
sqlc.yaml           sqlc generator configuration
```

## Required environment

- `DATABASE_URL` — Postgres connection string
- `ENCRYPTION_KEY` — 32-byte hex (64 hex characters)
- `SERVER_PORT` — optional, default `8080`
- `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` — optional
- `ON_CALL_PROVIDER` — optional, default `none`
- `ON_CALL_API_KEY` — optional
