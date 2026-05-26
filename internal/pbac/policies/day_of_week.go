package policies

import (
	"context"

	"github.com/aechrok/warden/internal/pbac"
)

// DayOfWeekConfig configures the DayOfWeek policy. AllowedDays lists weekday
// numbers per time.Weekday (0 = Sunday … 6 = Saturday). An empty list denies
// every day; the typical "business days" config is {1,2,3,4,5}.
type DayOfWeekConfig struct {
	AllowedDays []int `json:"allowed_days"`
}

// DayOfWeek denies actions executed on a day not in the AllowedDays set.
type DayOfWeek struct {
	Config DayOfWeekConfig
}

// Name implements pbac.Policy.
func (DayOfWeek) Name() string { return "day_of_week" }

// Evaluate implements pbac.Policy.
func (p DayOfWeek) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	today := int(ec.Now.Weekday())
	for _, d := range p.Config.AllowedDays {
		if d == today {
			return pbac.Allow
		}
	}
	return pbac.Deny
}
