package policies

import (
	"context"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// IntegrationHealthCheck denies destructive actions against an integration
// instance whose last health probe failed. This prevents the half-applied
// state that often follows when the upstream API is degraded.
type IntegrationHealthCheck struct{}

// Name implements pbac.Policy.
func (IntegrationHealthCheck) Name() string { return "integration_health_check" }

// Evaluate implements pbac.Policy.
func (IntegrationHealthCheck) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if !ec.Destructive {
		return pbactypes.Allow
	}
	if !ec.IntegrationHealthy {
		return pbactypes.Deny
	}
	return pbactypes.Allow
}
