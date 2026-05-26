package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// SelfTargetingBlock denies any action where the acting operator is also
// the target. This guards against an operator locking themselves out, and
// against social-engineering scenarios where an attacker would otherwise
// be able to use the victim's own account to escalate or clean up.
type SelfTargetingBlock struct{}

// Name implements pbac.Policy.
func (SelfTargetingBlock) Name() string { return "self_targeting_block" }

// Evaluate implements pbac.Policy.
func (SelfTargetingBlock) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if ec.TargetIsSelf {
		return pbac.Deny
	}
	return pbac.Allow
}
