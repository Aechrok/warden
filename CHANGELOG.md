# Changelog

All notable changes to Warden are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased] — 2026-05-27

### Added

- **Local email/password authentication** — username + password is now the default login method. Bcrypt hashes stored in a new `password_hash` column on `users`. OIDC/SSO is optional and additive.
- **SSO settings** — new SSO & SCIM tab lets admins configure OIDC (issuer, client ID, client secret, redirect URL) directly in the UI. Client secret is encrypted at rest using the same key-encryption pattern as plugin credentials.
- **Enforce SSO** — checkbox in SSO settings that, when enabled, hides the password form on the login page entirely. The checkbox is only interactive when the currently logged-in admin authenticated via OIDC, preventing accidental lockout.
- **Magic link invitations** — admins can generate one-time login links for external users (auditors, etc.) from the new Invitations settings tab. Each link can carry an optional role assignment, optional label, and a configurable expiry (default 7 days). The link is revealed once on creation and invalidated after first use.
- **`/auth/config` endpoint** — public endpoint the login page calls on load to determine whether SSO is enabled and/or enforced, before the user authenticates.
- **`/auth/local` endpoint** — accepts email + password, verifies bcrypt hash, and issues a session cookie.
- **`/auth/magic` endpoint** — redeems a magic link token, upserts the user, assigns any configured role, marks the token used, and redirects to the dashboard.
- **User `origin` field** — tracks how each user last authenticated (`oidc`, `local`, `scim`). Used to gate the Enforce SSO control.
- **`is_builtin` flag on roles** — built-in roles (admin, auditor, operator) are now marked in the database and protected from deletion in the UI.
- **Settings: Invitations tab** — new tab (requires `users:write`) listing all magic link invitations with status (Pending / Used / Expired), expiry date, copy-link button for pending invitations, and delete for all.
- **Settings: Users tab** — extracted into its own component with set-password support; admins can set a local password for any user.
- **Settings: Groups tab** — SCIM Groups renamed from "SCIM Groups" to "Groups" and repositioned next to the Users tab.
- **Settings: SSO & SCIM tab** — consolidated view for OIDC configuration, SCIM base URL display, and SCIM token management (generate, list, revoke).
- **SCIM token isolation** — SCIM tokens (`scim:admin` scope) are now exclusively managed from the SSO & SCIM tab. They are generated with a descriptive name, revealed once on creation, and listed with per-token Revoke buttons.

### Changed

- **Login page** — redesigned to show the email/password form by default. If SSO is enabled, a "Continue with SSO" button appears below a divider. If SSO is enforced, the password form is hidden.
- **API Tokens tab** — `scim:admin` scope is filtered out of both the token list and the permissions checklist; SCIM tokens are managed exclusively from the SSO & SCIM tab.
- **OIDC is now optional** — the server starts cleanly when OIDC environment variables are absent. `auth.NewProvider` returns `nil` (not an error) in this case.

### Database migrations

| # | Description |
|---|---|
| 017 | `roles.is_builtin` boolean column |
| 018 | `users.origin` text column |
| 019 | `sso_config` singleton table (OIDC settings, encrypted secret) |
| 020 | `users.password_hash`, `sso_config.sso_enabled`, `sso_config.enforce_sso` |
| 021 | `magic_links` table (invitations) |

---

## [0.2.0] — 2026-05-26

### Added

- **Local development stack** (`make up` / `make down` / `make logs` / `make ps`) — `docker-compose.local.yml` + `Dockerfile.dev` bring up Postgres, Dex, and Warden from a single command on the host. The Vue SPA is baked into the image and served by the Go server; no separate Vite process required.
- Dex wired through `host.docker.internal` so browser and backend resolve the same OIDC issuer URL without hosts-file changes.

### Fixed

- pgx5 migrate driver required `pgx5://` URL scheme; corrected connection string construction.
- River schema migration (`rivermigrate.New`) now runs at boot before the River client starts. API signature corrected to single return value.
- OIDC issuer set to `localhost` with an internal dialer rewrite so the container can reach Dex via the host port.
- Local dev `admin@warden.dev` password hash corrected (bcrypt cost mismatch).
- PKCE removed from the relying-party flow — `rp.AuthURL` does not set a verifier cookie, causing callback failures.
- Hardcoded nonce removed from auth URL; exchange errors are now logged.
- First login user is immediately bootstrapped as admin and assigned the `admin@warden.dev` role.
- `vip_identities.label` column renamed to `reason` to match handler queries (migration 016).
- `AuditView` TypeScript hover style corrected.
- WCAG 2.1 AA accessibility fixes for `BreakGlassView` (contrast ratios, focus indicators, ARIA labels).

---

## [0.1.0] — 2026-05-26

Initial release — full stack built across eight parallel agents.

### Added

- **Foundation** — Go module, PostgreSQL 16 schema (migrations 001–015), domain types, configuration, structured logging.
- **Auth & access control** — OIDC relying-party login via Dex, session cookies, RBAC (roles + permissions), PBAC engine with five policy types (IP allowlist, time window, new operator probation, require reason, require approval), break-glass emergency access with post-hoc review, SCIM 2.0 user/group provisioning.
- **Plugin system** — credential resolver with encrypted secret storage, plugin interface, and seven integrations: GitHub, Google Workspace, Okta, AWS IAM, Slack, Jira, PagerDuty.
- **Legal hold engine** — hold lifecycle (active → released / expired), custodian management, cascade state machine that fans out to integration plugins via River async workers.
- **HTTP API** — internal admin API (`/api/v1/internal/`) and public API (`/api/v1/`), Chi router, JWT/session middleware, rate limiting, structured error responses.
- **Vue 3 SPA** — dashboard, hold management, identity search, action execution, approval queue, break-glass, audit log, and all settings tabs. Dark/light theme, mobile-responsive layout, permission-gated navigation.
- **Operability** — Dockerfile, Helm chart, GitHub Actions CI/CD pipeline, devcontainer, `warden-ctl` CLI, `Makefile`.
- **Quality** — security review (OWASP Top 10), unit tests, integration tests, E2E tests (Playwright).
