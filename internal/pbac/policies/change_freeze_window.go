package policies

import (
	"context"
	"time"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// FreezeWindow is one closed-open interval [Start, End) during which the
// ChangeFreezeWindow policy denies writes.
type FreezeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ChangeFreezeWindowConfig is the policy configuration.
type ChangeFreezeWindowConfig struct {
	Windows []FreezeWindow `json:"windows"`
	// AppliesToDestructiveOnly limits the policy to actions tagged
	// destructive. When false, the policy denies every action during a
	// freeze window — useful for hard release-freeze events.
	AppliesToDestructiveOnly bool `json:"applies_to_destructive_only"`
}

// ChangeFreezeWindow denies actions inside any configured freeze window.
// Break-glass invocations bypass this policy at a higher layer (the
// break-glass service does not run the PBAC engine).
type ChangeFreezeWindow struct {
	Config ChangeFreezeWindowConfig
}

// Name implements pbac.Policy.
func (ChangeFreezeWindow) Name() string { return "change_freeze_window" }

// Evaluate implements pbac.Policy.
func (p ChangeFreezeWindow) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if p.Config.AppliesToDestructiveOnly && !ec.Destructive {
		return pbactypes.Allow
	}
	for _, w := range p.Config.Windows {
		if w.Start.IsZero() || w.End.IsZero() {
			continue
		}
		if !ec.Now.Before(w.Start) && ec.Now.Before(w.End) {
			return pbactypes.Deny
		}
	}
	return pbactypes.Allow
}
