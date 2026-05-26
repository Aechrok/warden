package policies

import (
	"context"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// ConcurrentSessionLimitConfig configures the ConcurrentSessionLimit policy.
// A MaxSessions of 0 disables the policy.
type ConcurrentSessionLimitConfig struct {
	MaxSessions int `json:"max_sessions"`
}

// ConcurrentSessionLimit denies actions when the operator has more active
// sessions than the configured threshold. A high session count is a
// classic indicator of credential compromise (the attacker logged in from
// a second browser without invalidating the legitimate session).
type ConcurrentSessionLimit struct {
	Config ConcurrentSessionLimitConfig
}

// Name implements pbac.Policy.
func (ConcurrentSessionLimit) Name() string { return "concurrent_session_limit" }

// Evaluate implements pbac.Policy.
func (p ConcurrentSessionLimit) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if p.Config.MaxSessions <= 0 {
		return pbactypes.Allow
	}
	if ec.SessionCount > p.Config.MaxSessions {
		return pbactypes.Deny
	}
	return pbactypes.Allow
}
