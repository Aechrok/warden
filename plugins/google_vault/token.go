package google_vault

import (
	"context"

	"github.com/aechrok/warden/plugins/internal/googleauth"
)

// liveToken is the production tokenFunc. Tests override the plugin's tokenFn.
func liveToken(ctx context.Context, sa []byte, adminEmail string, scopes []string) (string, error) {
	return googleauth.AccessToken(ctx, sa, adminEmail, scopes)
}
