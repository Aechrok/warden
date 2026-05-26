// Package api provides the HTTP server surface for Warden, including the
// internal (session-auth) and public (bearer-token) API surfaces, OIDC auth
// flow, and SCIM 2.0 endpoint.
package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/breakglass"
	"github.com/aechrok/warden/internal/config"
	"github.com/aechrok/warden/internal/legalhold"
	"github.com/aechrok/warden/internal/pbac"
	"github.com/aechrok/warden/internal/plugin"
	"github.com/aechrok/warden/internal/rbac"
	"github.com/aechrok/warden/internal/store"
)

// Server holds all wired dependencies for the HTTP layer.
type Server struct {
	pool        *pgxpool.Pool
	cfg         *config.Config
	logger      *zap.Logger
	auth        *auth.Provider
	sessions    *auth.SessionStore
	users       *auth.UserStore
	checker     *rbac.Checker
	pbacEngine  *pbac.Engine
	oncall      pbac.OnCallResolver
	dispatcher  *plugin.Dispatcher
	holdSvc     *legalhold.Service
	breakglass  *breakglass.Service
	eventStore  *store.EventStore
	riverClient *river.Client[pgx.Tx]
}

// NewServer constructs and wires every service layer. It starts the River
// client (background job processing) before returning. The caller should
// cancel ctx (or call river.Client.Stop) during shutdown.
func NewServer(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, logger *zap.Logger) (*Server, error) {
	if pool == nil {
		return nil, fmt.Errorf("api: nil pool")
	}
	if cfg == nil {
		return nil, fmt.Errorf("api: nil config")
	}
	if logger == nil {
		return nil, fmt.Errorf("api: nil logger")
	}

	authProvider, err := auth.NewProvider(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("api: oidc provider: %w", err)
	}

	eventStore := store.NewEventStore()
	outboxStore := store.NewOutboxStore()
	registry := plugin.NewRegistry()
	resolver := plugin.NewCredentialResolver(pool, cfg.EncryptionKey)
	dispatcher := plugin.NewDispatcher(registry, resolver, pool)

	holdSvc := legalhold.NewService(pool, eventStore, outboxStore, registry)
	riverClient, err := holdSvc.NewRiverClient()
	if err != nil {
		return nil, fmt.Errorf("api: river client: %w", err)
	}
	holdSvc.SetRiverClient(riverClient)

	bgSvc := breakglass.NewService(pool, eventStore)

	policies, err := pbac.LoadFromDB(ctx, pool)
	if err != nil {
		logger.Warn("api: could not load PBAC policies from DB; using defaults", zap.Error(err))
		policies = pbac.DefaultPolicies()
	}
	if len(policies) == 0 {
		policies = pbac.DefaultPolicies()
	}
	pbacEngine := pbac.NewEngine(policies)

	oncallResolver := pbac.NewResolver(cfg)

	s := &Server{
		pool:        pool,
		cfg:         cfg,
		logger:      logger,
		auth:        authProvider,
		sessions:    auth.NewSessionStore(),
		users:       auth.NewUserStore(),
		checker:     rbac.NewChecker(),
		pbacEngine:  pbacEngine,
		oncall:      oncallResolver,
		dispatcher:  dispatcher,
		holdSvc:     holdSvc,
		breakglass:  bgSvc,
		eventStore:  eventStore,
		riverClient: riverClient,
	}

	if err := riverClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("api: start river client: %w", err)
	}

	return s, nil
}

// secureCookies returns true when the config suggests a production environment
// (OIDC issuer uses HTTPS).
func (s *Server) secureCookies() bool {
	return strings.HasPrefix(strings.ToLower(s.cfg.OIDCIssuer), "https://")
}
