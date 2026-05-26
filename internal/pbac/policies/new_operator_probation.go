package policies

import (
	"context"
	"time"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
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
func (p NewOperatorProbation) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if !ec.Destructive {
		return pbactypes.Allow
	}
	if p.Config.ProbationDays <= 0 {
		return pbactypes.Allow
	}
	threshold := time.Duration(p.Config.ProbationDays) * 24 * time.Hour
	if ec.OperatorTenure < threshold {
		return pbactypes.Deny
	}
	return pbactypes.Allow
}
