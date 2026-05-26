# Warden — Dev Setup

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (or Docker Engine + Compose v2 on Linux)
- [VS Code](https://code.visualstudio.com/) with the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)

No local Go, Node, or database installation is required — all tooling runs inside the dev container.

## Steps

1. **Clone the repository:**

   ```bash
   git clone https://github.com/Aechrok/warden.git
   cd warden
   ```

2. **Open in VS Code:**

   ```bash
   code .
   ```

   When prompted, click **"Reopen in Container"**. VS Code will build the dev container image (first run takes a few minutes), start PostgreSQL and Dex (mock OIDC), and install frontend dependencies.

   Alternatively, open the Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`) and run **"Dev Containers: Reopen in Container"**.

3. **Run migrations:**

   Open a terminal inside VS Code (it opens inside the container) and run:

   ```bash
   make migrate
   ```

4. **Start the dev server:**

   ```bash
   make dev
   ```

   This starts `air` — the Go hot-reload watcher. The server rebuilds automatically on file changes.

5. **Start the frontend dev server (optional, separate terminal):**

   ```bash
   cd frontend && npm run dev
   ```

6. **Open the app:**

   - **Vite dev server** (hot module replacement): http://localhost:5173
   - **Go server directly**: http://localhost:8080

   Login with the test account:
   - Email: `admin@warden.dev`
   - Password: `warden`

## Useful commands

| Command | Description |
|---------|-------------|
| `make dev` | Start Go hot-reload server |
| `make test` | Run all Go tests |
| `make lint` | Run golangci-lint |
| `make build` | Build production binaries to `bin/` |
| `make migrate` | Apply pending DB migrations |
| `make migrate-create NAME=foo` | Create a new migration pair |
| `make generate` | Regenerate sqlc + buf output |
| `make coverage` | Run tests with coverage report |

## Port forwarding

The devcontainer forwards these ports to your host automatically:

| Port | Service |
|------|---------|
| 8080 | Warden API server |
| 5173 | Vite dev server |
| 5432 | PostgreSQL |

## Environment

All environment variables are pre-configured in `.devcontainer/docker-compose.yml` for local development. The `ENCRYPTION_KEY` is set to a safe all-zeros key — do not use it in production.

The Dex OIDC provider runs at `http://localhost:5556/dex` and is pre-configured with a test user.
