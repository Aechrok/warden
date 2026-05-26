package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// TimeOfDayConfig configures the TimeOfDay policy. StartHour and EndHour are
// in 24h local time (interpreted in ec.Now's location). The window is
// inclusive of StartHour and exclusive of EndHour, e.g. (9, 18) permits
// 09:00:00 through 17:59:59. Windows that wrap midnight (Start > End) are
// supported.
type TimeOfDayConfig struct {
	StartHour            int  `json:"start_hour"`
	EndHour              int  `json:"end_hour"`
	ApplyToDestructiveOnly bool `json:"apply_to_destructive_only"`
	// RequireApprovalOutside controls behavior outside the window. When true
	// the policy returns RequireApproval; when false it returns Deny.
	RequireApprovalOutside bool `json:"require_approval_outside"`
}

// TimeOfDay returns Allow when ec.Now falls inside the configured window,
// otherwise Deny or RequireApproval depending on configuration.
type TimeOfDay struct {
	Config TimeOfDayConfig
}

// Name implements pbac.Policy.
func (TimeOfDay) Name() string { return "time_of_day" }

// Evaluate implements pbac.Policy.
func (p TimeOfDay) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if p.Config.ApplyToDestructiveOnly && !ec.Destructive {
		return pbac.Allow
	}
	hour := ec.Now.Hour()
	start, end := normalizeHour(p.Config.StartHour), normalizeHour(p.Config.EndHour)
	if inHourWindow(hour, start, end) {
		return pbac.Allow
	}
	if p.Config.RequireApprovalOutside {
		return pbac.RequireApproval
	}
	return pbac.Deny
}

// normalizeHour clamps an hour into 0..23 by modulo. Negative values wrap.
func normalizeHour(h int) int {
	h %= 24
	if h < 0 {
		h += 24
	}
	return h
}

// inHourWindow reports whether hour lies in [start, end), supporting
// windows that wrap midnight.
func inHourWindow(hour, start, end int) bool {
	if start == end {
		// Empty window: nothing is permitted.
		return false
	}
	if start < end {
		return hour >= start && hour < end
	}
	// Wraps midnight, e.g. 22..6 — permitted hours are [22..24) ∪ [0..6).
	return hour >= start || hour < end
}
