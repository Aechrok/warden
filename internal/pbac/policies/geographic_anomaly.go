package policies

import (
	"context"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// GeographicAnomalyConfig configures the GeographicAnomaly policy.
// AllowedRegions is matched case-sensitively against ec.GeoRegion (e.g.
// "US-CA", "DE"). An empty AllowedRegions list disables the policy.
type GeographicAnomalyConfig struct {
	AllowedRegions []string `json:"allowed_regions"`
}

// GeographicAnomaly denies actions originating from a region not in the
// allowlist. When ec.GeoRegion is empty (geolocation unavailable) the
// policy is a no-op — failing open is the documented behavior because the
// usual cause is a privacy or network configuration that the operator has
// no control over.
type GeographicAnomaly struct {
	Config GeographicAnomalyConfig
}

// Name implements pbac.Policy.
func (GeographicAnomaly) Name() string { return "geographic_anomaly" }

// Evaluate implements pbac.Policy.
func (p GeographicAnomaly) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if ec.GeoRegion == "" {
		return pbactypes.Allow
	}
	if len(p.Config.AllowedRegions) == 0 {
		return pbactypes.Allow
	}
	for _, r := range p.Config.AllowedRegions {
		if r == ec.GeoRegion {
			return pbactypes.Allow
		}
	}
	return pbactypes.Deny
}
