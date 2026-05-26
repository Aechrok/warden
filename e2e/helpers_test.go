// Package e2e contains integration tests that start the full Warden HTTP server
// against a real Postgres testcontainer and exercise the full request path.
package e2e_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.uber.org/zap"

	apiserver "github.com/aechrok/warden/internal/api"
	"github.com/aechrok/warden/internal/auth"
	"github.com/aechrok/warden/internal/config"
	"github.com/aechrok/warden/internal/rbac"
)

// httpClient does not follow redirects.
var httpClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Timeout: 10 * time.Second,
}

// startTestDB starts a Postgres 16 testcontainer with all migrations applied
// and returns the pool + connection string.
func startTestDB(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	ctx := context.Background()

	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "internal", "db", "migrations")

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("e2e: read migrations dir: %v", err)
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
		t.Skipf("e2e: testcontainers unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("e2e: connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("e2e: pgxpool new: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("e2e: ping: %v", err)
	}
	return pool, connStr
}

// startServer starts the Warden HTTP server on a random port and returns its
// base URL and the database pool.
func startServer(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	pool, connStr := startTestDB(t)
	ctx := context.Background()

	cfg := &config.Config{
		DatabaseURL:   connStr,
		ServerPort:    0,
		EncryptionKey: make([]byte, 32),
		OIDCIssuer:    "http://localhost:0",
	}

	logger := zap.NewNop()
	srv, err := apiserver.NewServer(ctx, pool, cfg, logger)
	if err != nil {
		t.Skipf("e2e: server init failed (OIDC discovery unavailable): %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("e2e: listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpSrv.Serve(ln) }()
	t.Cleanup(func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx2)
	})

	// Wait for the server to start accepting connections.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	return baseURL, pool
}

// tokenHash computes the SHA-256 hex hash of a token value, matching the
// implementation in middleware/token.go.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// makeToken creates an API token for a new user with the given scopes and
// returns the raw token string.
func makeToken(t *testing.T, pool *pgxpool.Pool, scopes []string) string {
	t.Helper()
	ctx := context.Background()
	us := auth.NewUserStore()
	checker := rbac.NewChecker()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("makeToken begin: %v", err)
	}
	email := fmt.Sprintf("token_user_%s@example.com", uuid.New().String()[:8])
	user, err := us.GetOrCreate(ctx, tx, email, "Token User")
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("makeToken GetOrCreate: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("makeToken commit user: %v", err)
	}

	// Ensure admin role exists with all permissions.
	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO roles (name, description) VALUES ('admin','Admin')
		ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description
		RETURNING id
	`).Scan(&roleID); err != nil {
		t.Fatalf("makeToken create role: %v", err)
	}
	for _, p := range rbac.AllPermissions() {
		_, _ = pool.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roleID, string(p))
	}
	if err := checker.AssignRole(ctx, pool, user.ID, roleID, uuid.Nil); err != nil {
		t.Fatalf("makeToken assign role: %v", err)
	}

	rawToken := fmt.Sprintf("warden_test_%s", uuid.New().String())
	hash := tokenHash(rawToken)
	if _, err := pool.Exec(ctx, `
		INSERT INTO api_tokens (user_id, name, token_hash, scopes)
		VALUES ($1, $2, $3, $4)
	`, user.ID, "e2e-token", hash, scopes); err != nil {
		t.Fatalf("makeToken insert: %v", err)
	}
	return rawToken
}

// postJSON makes a JSON POST request with optional bearer token and returns the response.
func postJSON(t *testing.T, url, token string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("postJSON marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("postJSON new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("postJSON do: %v", err)
	}
	return resp
}

// getReq makes a GET request with optional bearer token and returns the response.
func getReq(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("getReq new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("getReq do: %v", err)
	}
	return resp
}

// decodeJSON decodes the response body into v and closes the body.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
}

// assertStatus fails the test if resp.StatusCode != want.
func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body = %v", resp.StatusCode, want, body)
	}
}
