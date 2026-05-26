package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// BulkActionThresholdConfig configures the BulkActionThreshold policy. The
// rolling window is owned by the caller (the action dispatcher counts
// recent executions and writes ec.BulkCount); the policy is just a
// threshold check.
type BulkActionThresholdConfig struct {
	MaxCount      int `json:"max_count"`
	WindowMinutes int `json:"window_minutes"`
}

// BulkActionThreshold denies when the same action has been executed by the
// acting operator more than MaxCount times within the configured rolling
// window. The window itself is informational (used by the dispatcher when
// computing BulkCount); this policy simply compares the count.
type BulkActionThreshold struct {
	Config BulkActionThresholdConfig
}

// Name implements pbac.Policy.
func (BulkActionThreshold) Name() string { return "bulk_action_threshold" }

// Evaluate implements pbac.Policy.
func (p BulkActionThreshold) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if p.Config.MaxCount <= 0 {
		return pbac.Allow
	}
	if ec.BulkCount > p.Config.MaxCount {
		return pbac.Deny
	}
	return pbac.Allow
}
