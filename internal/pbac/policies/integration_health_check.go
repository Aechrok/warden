package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// IntegrationHealthCheck denies destructive actions against an integration
// instance whose last health probe failed. This prevents the half-applied
// state that often follows when the upstream API is degraded.
type IntegrationHealthCheck struct{}

// Name implements pbac.Policy.
func (IntegrationHealthCheck) Name() string { return "integration_health_check" }

// Evaluate implements pbac.Policy.
func (IntegrationHealthCheck) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if !ec.Destructive {
		return pbac.Allow
	}
	if !ec.IntegrationHealthy {
		return pbac.Deny
	}
	return pbac.Allow
}
