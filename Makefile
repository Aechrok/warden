.PHONY: dev migrate migrate-create generate test lint build ctl-apply coverage

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
