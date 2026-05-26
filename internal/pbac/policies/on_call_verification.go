package policies

import (
	"context"
	"path/filepath"

	"github.com/aechrok/warden/internal/pbac"
)

// OnCallVerificationConfig configures the OnCallVerification policy.
// RequiredFor lists action-key glob patterns (filepath.Match) for which
// on-call membership is required. An empty list disables the policy.
type OnCallVerificationConfig struct {
	RequiredFor []string `json:"required_for"`
}

// OnCallVerification denies high-impact actions when the operator is not
// currently on the on-call roster. ec.OnCall is sourced from the
// configured OnCallResolver (PagerDuty, OpsGenie, or none).
type OnCallVerification struct {
	Config OnCallVerificationConfig
}

// Name implements pbac.Policy.
func (OnCallVerification) Name() string { return "on_call_verification" }

// Evaluate implements pbac.Policy.
func (p OnCallVerification) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if len(p.Config.RequiredFor) == 0 {
		return pbac.Allow
	}
	if !matchesAnyPattern(p.Config.RequiredFor, ec.ActionKey) {
		return pbac.Allow
	}
	if ec.OnCall {
		return pbac.Allow
	}
	return pbac.Deny
}

func matchesAnyPattern(patterns []string, s string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		ok, err := filepath.Match(p, s)
		if err == nil && ok {
			return true
		}
	}
	return false
}
