package pbac

import "github.com/aechrok/warden/internal/pbac/types"

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
type Result = types.Result

const (
	// Allow permits the action to proceed.
	Allow = types.Allow
	// RequireApproval places the action into the approval queue.
	RequireApproval = types.RequireApproval
	// Deny rejects the action outright.
	Deny = types.Deny
	// Override is the incident-expansion short circuit: when any policy
	// returns Override, the engine returns Allow regardless of other
	// policies' results. Only incident_window_expansion uses this.
	Override = types.Override
)

// Combine returns the more restrictive of a and b, except that Override
// short-circuits the result to Allow.
func Combine(a, b Result) Result { return types.Combine(a, b) }

// Policy is the unit of evaluation. Implementations should be stateless and
// safe for concurrent use: the engine evaluates the same Policy instance
// across many requests.
type Policy = types.Policy
