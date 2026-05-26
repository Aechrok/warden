# Warden — Deployment Guide

## Docker Compose (Single-Node)

The `docker-compose.prod.yml` at the repository root provides a reference single-node deployment with Warden, PostgreSQL 16, and Caddy (automatic TLS via Let's Encrypt).

### Prerequisites

- Docker Engine 24+ with Compose v2
- A domain name with DNS pointing to the host

### Steps

1. Copy and edit the environment file:

```bash
cp .env.example .env
# Fill in all values — see Environment Variables table below
```

2. Pull and start:

```bash
docker compose -f docker-compose.prod.yml up -d
```

3. Run migrations (first deploy only):

```bash
docker compose -f docker-compose.prod.yml exec warden \
  /warden migrate -path /internal/db/migrations -database "$DATABASE_URL" up
```

Alternatively, run the `golang-migrate` CLI directly:

```bash
docker run --rm --network host \
  migrate/migrate \
  -path /migrations \
  -database "postgres://warden:$POSTGRES_PASSWORD@localhost:5432/warden?sslmode=disable" \
  up
```

4. Open `https://<WARDEN_DOMAIN>` in your browser. Login is handled via OIDC.

---

## Helm (Kubernetes)

The Helm chart lives at `charts/warden/`. It requires Kubernetes 1.25+ and Helm 3.10+.

### Quick install

```bash
helm upgrade --install warden ./charts/warden \
  --namespace warden --create-namespace \
  --set env.DATABASE_URL="postgres://warden:secret@postgres:5432/warden?sslmode=disable" \
  --set env.ENCRYPTION_KEY="<32-byte-hex>" \
  --set env.OIDC_ISSUER="https://accounts.google.com" \
  --set env.OIDC_CLIENT_ID="<client-id>" \
  --set env.OIDC_CLIENT_SECRET="<client-secret>" \
  --set env.OIDC_REDIRECT_URL="https://warden.example.com/auth/callback" \
  --set ingress.enabled=true \
  --set ingress.host=warden.example.com
```

All secret env vars are stored in a Kubernetes Secret named `<release-name>-env`. Do not put them in `values.yaml` in plaintext; use `--set` or an external secrets manager (External Secrets Operator, Vault Agent, etc.).

### Notable values

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `2` | Number of pods |
| `autoscaling.enabled` | `false` | Enable HPA (2–10 replicas, 70% CPU) |
| `pdb.enabled` | `true` | Enable PodDisruptionBudget (minAvailable: 1) |
| `metrics.enabled` | `false` | Enable Prometheus ServiceMonitor |
| `ingress.enabled` | `false` | Enable Ingress |
| `ingress.host` | `warden.example.com` | Ingress hostname |
| `image.tag` | (chart appVersion) | Override image tag |

### Migrations in Kubernetes

Run migrations as a Job before the rollout:

```bash
kubectl run warden-migrate --image=ghcr.io/aechrok/warden:latest \
  --restart=Never --rm -it \
  --env="DATABASE_URL=$DATABASE_URL" \
  -- /warden migrate -path /internal/db/migrations -database "$DATABASE_URL" up
```

Or use an `initContainer` in the Deployment (not included by default — opt in via a custom `values.yaml` overlay).

---

## Environment Variable Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | PostgreSQL DSN, e.g. `postgres://user:pass@host:5432/dbname?sslmode=disable` |
| `ENCRYPTION_KEY` | Yes | — | 32-byte key, hex-encoded (64 hex chars). Used for credential encryption. |
| `SERVER_PORT` | No | `8080` | TCP port the HTTP server listens on. |
| `OIDC_ISSUER` | No | — | OIDC provider issuer URL (e.g. `https://accounts.google.com`). Required for SSO. |
| `OIDC_CLIENT_ID` | No | — | OIDC client ID. |
| `OIDC_CLIENT_SECRET` | No | — | OIDC client secret. |
| `OIDC_REDIRECT_URL` | No | — | OIDC redirect callback URL, must match provider config. |
| `ON_CALL_PROVIDER` | No | `none` | On-call verification provider: `none`, `pagerduty`, or `opsgenie`. |
| `ON_CALL_API_KEY` | No | — | API key for the on-call provider. |

### Generating ENCRYPTION_KEY

```bash
openssl rand -hex 32
```

Keep this value secret. Rotating it requires re-encrypting all stored credentials.
