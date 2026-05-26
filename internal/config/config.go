// Package config loads Warden's runtime configuration from environment
// variables. No third-party config library is used; the surface area is
// intentionally small and explicit.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration.
type Config struct {
	DatabaseURL    string
	ServerPort     int
	EncryptionKey  []byte
	OIDCIssuer     string
	OIDCClientID   string
	OIDCSecret     string
	OnCallProvider string
	OnCallAPIKey   string
}

// Load reads configuration from the process environment. Fails fast with a
// descriptive error if any required value is missing or malformed.
func Load() (*Config, error) {
	var missing []string

	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dbURL == "" {
		missing = append(missing, "DATABASE_URL")
	}

	encHex := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY"))
	if encHex == "" {
		missing = append(missing, "ENCRYPTION_KEY")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("config: missing required environment variable(s): %s", strings.Join(missing, ", "))
	}

	encKey, err := hex.DecodeString(encHex)
	if err != nil {
		return nil, fmt.Errorf("config: ENCRYPTION_KEY must be hex-encoded: %w", err)
	}
	if len(encKey) != 32 {
		return nil, fmt.Errorf("config: ENCRYPTION_KEY must decode to 32 bytes, got %d", len(encKey))
	}

	port := 8080
	if raw := strings.TrimSpace(os.Getenv("SERVER_PORT")); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("config: SERVER_PORT must be an integer: %w", err)
		}
		if p <= 0 || p > 65535 {
			return nil, errors.New("config: SERVER_PORT must be between 1 and 65535")
		}
		port = p
	}

	onCallProvider := strings.TrimSpace(os.Getenv("ON_CALL_PROVIDER"))
	if onCallProvider == "" {
		onCallProvider = "none"
	}

	return &Config{
		DatabaseURL:    dbURL,
		ServerPort:     port,
		EncryptionKey:  encKey,
		OIDCIssuer:     strings.TrimSpace(os.Getenv("OIDC_ISSUER")),
		OIDCClientID:   strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID")),
		OIDCSecret:     strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET")),
		OnCallProvider: onCallProvider,
		OnCallAPIKey:   strings.TrimSpace(os.Getenv("ON_CALL_API_KEY")),
	}, nil
}
