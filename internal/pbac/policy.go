package pbac

import "context"

// Result is the outcome of a single policy evaluation. The engine combines
// per-policy results using the most-restrictive rule with one exception:
// an Override result short-circuits to Allow regardless of other policies.
// Override exists for incident_window_expansion, which by design expands
// operator permissions during a declared incident.
//
//	Deny > RequireApproval > Allow
//	Override → Allow (short-circuit, beats Deny)
//
// Allow is the zero value so a slice of policies that returns no opinion
// (all default) lands on Allow.
type Result int

const (
	// Allow permits the action to proceed.
	Allow Result = iota
	// RequireApproval places the action into the approval queue.
	RequireApproval
	// Deny rejects the action outright.
	Deny
	// Override is the incident-expansion short circuit: when any policy
	// returns Override, the engine returns Allow regardless of other
	// policies' results. Only incident_window_expansion uses this.
	Override
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

// Combine returns the more restrictive of a and b, except that Override
// short-circuits the result to Allow.
func Combine(a, b Result) Result {
	if a == Override || b == Override {
		return Override
	}
	if a > b {
		return a
	}
	return b
}

// Policy is the unit of evaluation. Implementations should be stateless and
// safe for concurrent use: the engine evaluates the same Policy instance
// across many requests.
type Policy interface {
	// Name is a stable identifier suitable for storage in pbac_policies.name
	// and for surfacing in audit events. Lowercase snake_case.
	Name() string

	// Evaluate returns the result for the supplied context. Implementations
	// must not mutate ec.
	Evaluate(ctx context.Context, ec EvalContext) Result
}
