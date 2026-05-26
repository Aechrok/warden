// Package testutil provides shared test helpers for database-backed tests.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// NewTestPool starts a Postgres 16 testcontainer, runs all migrations, and
// returns a pool. The container is terminated when the test completes.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// Locate the migrations directory relative to this source file.
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../internal/testutil/db.go
	// migrations are at .../internal/db/migrations/
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "db", "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("testutil: read migrations dir %s: %v", migrationsDir, err)
	}
	var scripts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			scripts = append(scripts, filepath.Join(migrationsDir, e.Name()))
		}
	}

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("warden_test"),
		tcpostgres.WithUsername("warden"),
		tcpostgres.WithPassword("warden"),
		tcpostgres.WithOrderedInitScripts(scripts...),
	)
	if err != nil {
		t.Skipf("testutil: testcontainers unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testutil: connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("testutil: pgxpool new: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("testutil: ping: %v", err)
	}
	return pool
}
