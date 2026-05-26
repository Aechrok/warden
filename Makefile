.PHONY: dev migrate migrate-create generate test lint build ctl-apply coverage up down logs ps

BINARY     := warden
CTL_BINARY := warden-ctl
GOFLAGS    :=

dev:
	air -c .air.toml

migrate:
	migrate -path internal/db/migrations -database "$(DATABASE_URL)" up

migrate-create:
	migrate create -ext sql -dir internal/db/migrations -seq $(NAME)

generate:
	go tool sqlc generate
	go tool buf generate

test:
	go test ./... -count=1

coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/server
	go build $(GOFLAGS) -o bin/$(CTL_BINARY) ./cmd/warden-ctl

ctl-apply:
	./bin/$(CTL_BINARY) apply --file $(FILE)

# ── Local stack (no devcontainer required) ────────────────────────────────────
# Access the app at http://localhost:8080 after `make up`.
# Login: admin@warden.dev / warden
#
# Note: host.docker.internal must resolve on your machine. Docker Desktop for
# Windows adds it automatically. If login redirects fail, verify it resolves.

up:
	docker compose -f docker-compose.local.yml up --build -d
	@echo ""
	@echo "  Warden is starting. Open http://localhost:8080 in your browser."
	@echo "  Login: admin@warden.dev  /  warden"
	@echo ""
	@echo "  Run 'make logs' to follow output, 'make down' to stop."

down:
	docker compose -f docker-compose.local.yml down

logs:
	docker compose -f docker-compose.local.yml logs -f

ps:
	docker compose -f docker-compose.local.yml ps
