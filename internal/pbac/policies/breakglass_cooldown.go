package policies

import (
	"context"
	"time"

	"github.com/aechrok/warden/internal/pbac"
)

// BreakGlassActionKey is the canonical action key the break-glass service
// uses when invoking emergency override. The policy is keyed off this exact
// string; other actions pass through.
const BreakGlassActionKey = "breakglass:invoke"

// BreakGlassCooldownConfig configures the cooldown window in hours. A value
// of 0 disables the policy.
type BreakGlassCooldownConfig struct {
	CooldownHours int `json:"cooldown_hours"`
}

// BreakGlassCooldown denies repeated break-glass invocations within the
// configured cooldown. Forces operators who repeatedly need emergency
// override to escalate to a peer instead of relying on the override path.
type BreakGlassCooldown struct {
	Config BreakGlassCooldownConfig
}

// Name implements pbac.Policy.
func (BreakGlassCooldown) Name() string { return "breakglass_cooldown" }

// Evaluate implements pbac.Policy.
func (p BreakGlassCooldown) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if ec.ActionKey != BreakGlassActionKey {
		return pbac.Allow
	}
	if p.Config.CooldownHours <= 0 || ec.LastBreakGlass == nil {
		return pbac.Allow
	}
	cooldown := time.Duration(p.Config.CooldownHours) * time.Hour
	if ec.Now.Sub(*ec.LastBreakGlass) < cooldown {
		return pbac.Deny
	}
	return pbac.Allow
}
