// Command server is the Warden control-plane HTTP/gRPC server entry point.
// At this stage it bootstraps configuration, opens the database pool, runs
// migrations, and blocks waiting for a termination signal. Subsequent agents
// will mount the API surfaces and background workers onto this scaffold.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/config"
	"github.com/aechrok/warden/internal/rbac"

	// Blank imports register every integration plugin via init().
	_ "github.com/aechrok/warden/plugins/google"
	_ "github.com/aechrok/warden/plugins/google_vault"
	_ "github.com/aechrok/warden/plugins/intune"
	_ "github.com/aechrok/warden/plugins/jamf"
	_ "github.com/aechrok/warden/plugins/m365"
	_ "github.com/aechrok/warden/plugins/okta"
	_ "github.com/aechrok/warden/plugins/slack"
)

const migrationsSource = "file://internal/db/migrations"

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		// At this point we cannot log via zap; fall back to stderr exit.
		os.Stderr.WriteString("warden: failed to construct logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	if err := run(logger); err != nil {
		logger.Fatal("warden: startup failed", zap.Error(err))
	}
}

func run(logger *zap.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return err
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("warden: migrations complete")

	if err := rbac.NewSeeder().Seed(ctx, pool); err != nil {
		return err
	}
	logger.Info("warden: built-in roles seeded")

	logger.Info("warden: ready", zap.Int("port", cfg.ServerPort))

	<-ctx.Done()
	logger.Info("warden: shutdown signal received")
	return nil
}

func runMigrations(databaseURL string) error {
	m, err := migrate.New(migrationsSource, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		srcErr, dbErr := m.Close()
		_ = srcErr
		_ = dbErr
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
