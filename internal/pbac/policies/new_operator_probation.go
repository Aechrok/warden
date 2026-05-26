package policies

import (
	"context"
	"time"

	"github.com/aechrok/warden/internal/pbac"
)

// NewOperatorProbationConfig configures the NewOperatorProbation policy.
// ProbationDays is the threshold in 24-hour days; values <= 0 disable the
// policy entirely.
type NewOperatorProbationConfig struct {
	ProbationDays int `json:"probation_days"`
}

// NewOperatorProbation denies destructive actions for operators whose
// accounts were created within the configured probation window. The intent
// is to give administrators a buffer in which the operator's training,
// hardware, and account hygiene are still being verified.
type NewOperatorProbation struct {
	Config NewOperatorProbationConfig
}

// Name implements pbac.Policy.
func (NewOperatorProbation) Name() string { return "new_operator_probation" }

// Evaluate implements pbac.Policy.
func (p NewOperatorProbation) Evaluate(_ context.Context, ec pbac.EvalContext) pbac.Result {
	if !ec.Destructive {
		return pbac.Allow
	}
	if p.Config.ProbationDays <= 0 {
		return pbac.Allow
	}
	threshold := time.Duration(p.Config.ProbationDays) * 24 * time.Hour
	if ec.OperatorTenure < threshold {
		return pbac.Deny
	}
	return pbac.Allow
}
