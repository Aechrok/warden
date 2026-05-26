package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
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
func (p ConcurrentSessionLimit) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if p.Config.MaxSessions <= 0 {
		return pbac.Allow
	}
	if ec.SessionCount > p.Config.MaxSessions {
		return pbac.Deny
	}
	return pbac.Allow
}
