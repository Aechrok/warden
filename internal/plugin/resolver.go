package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/crypto"
	"github.com/aechrok/warden/internal/domain"
)

// ErrInstanceNotFound is returned when an integration instance row cannot be
// located by ID.
var ErrInstanceNotFound = errors.New("plugin: integration instance not found")

// ErrPluginNotFound is returned when an instance references a plugin id that
// is not registered.
var ErrPluginNotFound = errors.New("plugin: plugin not registered")

// CredentialResolver decrypts credentials for an integration instance.
//
// Resolution order, per credential field:
//  1. Environment variable override (operator-controlled, never persisted).
//     Env var name format: {INSTANCE_NAME_UPPER}_{FIELD_KEY_UPPER}, where
//     non-alphanumeric characters in either component are replaced with `_`.
//     Example: instance "okta-prod", field "api_token"
//     -> env var "OKTA_PROD_API_TOKEN".
//  2. Database column `credentials_enc` (AES-256-GCM, JSON object of
//     {field_key: value}).
//
// Fields missing from both sources are simply omitted; required-field
// validation is the responsibility of the plugin's HealthCheck or action
// implementation.
type CredentialResolver struct {
	pool   *pgxpool.Pool
	encKey []byte
}

// NewCredentialResolver constructs a resolver bound to the given pool and
// 32-byte AES-256-GCM key.
func NewCredentialResolver(pool *pgxpool.Pool, encKey []byte) *CredentialResolver {
	return &CredentialResolver{pool: pool, encKey: encKey}
}

// InstanceRow is the minimal projection of integration_instances needed by
// the resolver and dispatcher. Exported so the dispatcher can reuse the row
// returned by LookupInstance without a second query.
type InstanceRow struct {
	ID             uuid.UUID
	Name           string
	PluginID       string
	CredentialsEnc []byte
	IsActive       bool
}

// LookupInstance fetches the instance row by ID. Returns ErrInstanceNotFound
// if no such row exists.
func (r *CredentialResolver) LookupInstance(ctx context.Context, instanceID uuid.UUID) (InstanceRow, error) {
	var row InstanceRow
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, plugin_id, credentials_enc, is_active
		FROM integration_instances
		WHERE id = $1
	`, instanceID).Scan(&row.ID, &row.Name, &row.PluginID, &row.CredentialsEnc, &row.IsActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InstanceRow{}, ErrInstanceNotFound
		}
		return InstanceRow{}, fmt.Errorf("plugin resolver: lookup instance: %w", err)
	}
	return row, nil
}

// Resolve decrypts the instance's stored credentials and overlays any
// matching environment-variable overrides. The returned Credentials map
// contains every key found in either source; absent keys are not present.
func (r *CredentialResolver) Resolve(ctx context.Context, instanceID uuid.UUID) (domain.Credentials, error) {
	row, err := r.LookupInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return r.resolveFromRow(row)
}

// ResolveRow is the variant that avoids re-querying the instance when the
// caller already has the row in hand.
func (r *CredentialResolver) ResolveRow(row InstanceRow) (domain.Credentials, error) {
	return r.resolveFromRow(row)
}

func (r *CredentialResolver) resolveFromRow(row InstanceRow) (domain.Credentials, error) {
	creds := domain.Credentials{}
	if len(row.CredentialsEnc) > 0 {
		pt, err := crypto.Decrypt(r.encKey, row.CredentialsEnc)
		if err != nil {
			return nil, fmt.Errorf("plugin resolver: decrypt credentials: %w", err)
		}
		// Stored shape is a flat JSON object {key: value}.
		raw := map[string]string{}
		if err := json.Unmarshal(pt, &raw); err != nil {
			return nil, fmt.Errorf("plugin resolver: unmarshal credentials: %w", err)
		}
		for k, v := range raw {
			creds[k] = v
		}
	}
	// Environment overrides for known keys, plus any extra keys the operator
	// may have set without a corresponding DB entry.
	prefix := envPrefix(row.Name)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, prefix+"_") {
			continue
		}
		eq := strings.IndexByte(env, '=')
		if eq < 0 {
			continue
		}
		name := env[:eq]
		val := env[eq+1:]
		fieldKey := strings.ToLower(strings.TrimPrefix(name, prefix+"_"))
		if fieldKey == "" {
			continue
		}
		creds[fieldKey] = val
	}
	return creds, nil
}

// envPrefix derives the {INSTANCE_NAME_UPPER} portion of the override env
// variable name. Non-alphanumeric characters become `_`.
func envPrefix(instanceName string) string {
	var b strings.Builder
	b.Grow(len(instanceName))
	for _, r := range instanceName {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
