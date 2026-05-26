package policies

import (
	"context"
	"time"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// StepUpMFAConfig configures the StepUpMFA policy. MaxSessionAgeMinutes is
// the upper bound on session age, in minutes, before a destructive action
// requires re-authentication via the approval queue (which the front-end
// resolves with an interstitial re-auth prompt).
type StepUpMFAConfig struct {
	MaxSessionAgeMinutes int `json:"max_session_age_minutes"`
}

// StepUpMFA requires approval (used by the UI as a re-authentication prompt)
// when the operator's session is older than the configured threshold and
// the action is destructive.
type StepUpMFA struct {
	Config StepUpMFAConfig
}

// Name implements pbac.Policy.
func (StepUpMFA) Name() string { return "step_up_mfa" }

// Evaluate implements pbac.Policy.
func (p StepUpMFA) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if !ec.Destructive {
		return pbactypes.Allow
	}
	if p.Config.MaxSessionAgeMinutes <= 0 {
		return pbactypes.Allow
	}
	threshold := time.Duration(p.Config.MaxSessionAgeMinutes) * time.Minute
	if ec.SessionAge > threshold {
		return pbactypes.RequireApproval
	}
	return pbactypes.Allow
}
