// Package types contains the shared types used by both the pbac engine and
// the pbac/policies sub-package. It exists solely to break the import cycle:
//
//	pbac → pbac/policies → pbac  (cycle)
//
// By placing the shared types here, both packages import pbac/types instead.
package types

import (
	"context"
	"net"
	"time"
)

// EvalContext is the snapshot of runtime context passed to every policy.
type EvalContext struct {
	Now            time.Time
	SourceIP       net.IP
	GeoRegion      string
	SessionAge     time.Duration
	SessionCount   int
	OperatorTenure time.Duration
	OnCall         bool
	TargetEmail    string
	TargetIsVIP    bool
	TargetIsSelf   bool
	InstanceName   string
	IntegrationHealthy bool
	ActionKey      string
	Destructive    bool
	ActiveIncident bool
	BulkCount      int
	ActiveHoldCount int
	LastBreakGlass *time.Time
}

// Result is the outcome of a single policy evaluation.
type Result int

const (
	Allow          Result = iota
	RequireApproval Result = iota
	Deny           Result = iota
	Override       Result = iota
)

// String renders the result for logs and events.
func (r Result) String() string {
	switch r {
	case Allow:
		return "allow"
	case RequireApproval:
		return "require_approval"
	case Deny:
		return "deny"
	case Override:
		return "override_allow"
	default:
		return "unknown"
	}
}

// Combine returns the more restrictive of a and b.
func Combine(a, b Result) Result {
	if a == Override || b == Override {
		return Override
	}
	if a > b {
		return a
	}
	return b
}

// Policy is the unit of evaluation.
type Policy interface {
	Name() string
	Evaluate(ctx context.Context, ec EvalContext) Result
}
