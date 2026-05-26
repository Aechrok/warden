package google

import (
	"context"
	"time"

	"github.com/aechrok/warden/plugins/internal/googleauth"
)

// liveToken is the production tokenFunc. It is overridden by tests with a
// stub that returns a fixed access token without contacting Google.
func liveToken(ctx context.Context, sa []byte, adminEmail string, scopes []string) (string, error) {
	return googleauth.AccessToken(ctx, sa, adminEmail, scopes)
}

// nowFn is the time source used by the plugin. Replaced by tests for
// deterministic FetchedAt timestamps.
var nowFn = time.Now
