// Command server is the Warden control-plane HTTP server entry point.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/api"
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
		return fmt.Errorf("open db pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	logger.Info("warden: migrations complete")

	if err := rbac.NewSeeder().Seed(ctx, pool); err != nil {
		return fmt.Errorf("rbac seed: %w", err)
	}
	logger.Info("warden: built-in roles seeded")

	srv, err := api.NewServer(ctx, pool, cfg, logger)
	if err != nil {
		return fmt.Errorf("api server init: %w", err)
	}

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServerPort),
		Handler: srv.Handler(),
	}

	go func() {
		logger.Info("warden: listening", zap.Int("port", cfg.ServerPort))
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("warden: http server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("warden: shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("warden: graceful shutdown error", zap.Error(err))
	}
	logger.Info("warden: shutdown complete")
	return nil
}

func runMigrations(databaseURL string) error {
	// The pgx/v5 migrate driver registers under "pgx5://" not "postgres://".
	// Rewrite the scheme so migrate finds its driver while the pool URL is unchanged.
	migrateURL := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	m, err := migrate.New(migrationsSource, migrateURL)
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
