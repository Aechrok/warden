package policies

import (
	"context"
	"path/filepath"

	"github.com/aechrok/warden/internal/pbac"
)

// ProductionInstanceGateConfig configures the ProductionInstanceGate policy.
// ProdPattern is a glob (filepath.Match syntax) tested against the instance
// name; the documented default is "*-prod".
type ProductionInstanceGateConfig struct {
	ProdPattern string `json:"prod_pattern"`
}

// ProductionInstanceGate requires approval for destructive actions against
// instances whose name matches the production glob. The intent is to keep
// the staging surface frictionless while raising the bar for prod.
type ProductionInstanceGate struct {
	Config ProductionInstanceGateConfig
}

// Name implements pbac.Policy.
func (ProductionInstanceGate) Name() string { return "production_instance_gate" }

// Evaluate implements pbac.Policy.
func (p ProductionInstanceGate) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if !ec.Destructive || ec.InstanceName == "" {
		return pbac.Allow
	}
	pattern := p.Config.ProdPattern
	if pattern == "" {
		pattern = "*-prod"
	}
	ok, err := filepath.Match(pattern, ec.InstanceName)
	if err != nil {
		// Malformed glob: fall back to substring match on "prod" so the
		// policy is not silently disabled by a typo.
		ok = containsProdHint(ec.InstanceName)
	}
	if ok {
		return pbac.RequireApproval
	}
	return pbac.Allow
}

func containsProdHint(name string) bool {
	for i := 0; i+4 <= len(name); i++ {
		if name[i] == 'p' && name[i+1] == 'r' && name[i+2] == 'o' && name[i+3] == 'd' {
			return true
		}
	}
	return false
}
