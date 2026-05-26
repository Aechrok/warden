package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// VIPProtection requires approval for any action whose target is flagged as
// a VIP identity (C-suite, board, legal counsel). The intent is a hard
// two-operator rule for the highest-risk targets regardless of the acting
// operator's role.
type VIPProtection struct{}

// Name implements pbac.Policy.
func (VIPProtection) Name() string { return "vip_protection" }

// Evaluate implements pbac.Policy.
func (VIPProtection) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if ec.TargetIsVIP {
		return pbac.RequireApproval
	}
	return pbac.Allow
}
