module github.com/aechrok/warden

go 1.25

require (
	connectrpc.com/connect v1.16.2
	github.com/golang-migrate/migrate/v4 v4.17.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/riverqueue/river v0.11.4
	github.com/riverqueue/river/riverdriver/riverpgxv5 v0.11.4
	github.com/zitadel/oidc/v3 v3.27.0
	go.uber.org/zap v1.27.0
	golang.org/x/oauth2 v0.21.0
	google.golang.org/protobuf v1.34.2
)

// Tool dependencies managed via `go tool` (Go 1.24+). Run `go tool <name>`
// inside the dev container; the Makefile wraps these as `make generate`.
tool (
	github.com/sqlc-dev/sqlc/cmd/sqlc
	github.com/bufbuild/buf/cmd/buf
	github.com/air-verse/air
)
