package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// HoldReleaseActionKey is the canonical action key the legal-hold service
// emits when releasing a hold. The policy is keyed off this exact string;
// other hold actions are not constrained.
const HoldReleaseActionKey = "hold:release"

// LegalHoldConflict denies releasing a hold for a custodian who is still
// referenced by another active hold. ActiveHoldCount is the count *across
// all active holds* including the one currently being released — hence the
// "> 1" comparison (releasing the only hold is permitted; releasing one of
// two is denied because the custodian is still held elsewhere).
type LegalHoldConflict struct{}

// Name implements pbac.Policy.
func (LegalHoldConflict) Name() string { return "legal_hold_conflict" }

// Evaluate implements pbac.Policy.
func (LegalHoldConflict) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if ec.ActionKey != HoldReleaseActionKey {
		return pbac.Allow
	}
	if ec.ActiveHoldCount > 1 {
		return pbac.Deny
	}
	return pbac.Allow
}
