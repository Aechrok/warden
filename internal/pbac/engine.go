package pbac

import "context"

// Engine evaluates a fixed slice of policies against an EvalContext and
// returns the most restrictive Result. Order of policies in the slice does
// not affect correctness — the combine rule is commutative — but it does
// affect the diagnostic trail returned by EvaluateWithTrace.
type Engine struct {
	policies []Policy
}

// NewEngine constructs an Engine with the given enabled policies. The slice
// is copied so callers may continue to mutate their input without affecting
// the engine.
func NewEngine(policies []Policy) *Engine {
	cp := make([]Policy, len(policies))
	copy(cp, policies)
	return &Engine{policies: cp}
}

// Policies returns the engine's policies in declaration order. Returned
// slice must not be mutated by callers.
func (e *Engine) Policies() []Policy {
	return e.policies
}

// Evaluate runs every enabled policy and returns the most restrictive
// outcome. Engines with zero policies always Allow. The Override result
// returned by incident_window_expansion short-circuits the engine to Allow
// regardless of any prior Deny / RequireApproval — that is the documented
// "expand permissions during declared incidents" semantics.
func (e *Engine) Evaluate(ctx context.Context, ec EvalContext) Result {
	worst := Allow
	for _, p := range e.policies {
		r := p.Evaluate(ctx, ec)
		if r == Override {
			return Allow
		}
		worst = Combine(worst, r)
	}
	return worst
}

// PolicyOutcome pairs a policy with the result it returned during a
// traced evaluation.
type PolicyOutcome struct {
	Policy string `json:"policy"`
	Result string `json:"result"`
}

// EvaluateWithTrace runs every policy and returns the overall result plus
// per-policy outcomes. Used by the audit log and the debug endpoint to
// explain why an action was blocked. Override behaves exactly as in
// Evaluate: the overall result becomes Allow, while the trace preserves
// every per-policy outcome (including the Override entry).
func (e *Engine) EvaluateWithTrace(ctx context.Context, ec EvalContext) (Result, []PolicyOutcome) {
	trace := make([]PolicyOutcome, 0, len(e.policies))
	worst := Allow
	overridden := false
	for _, p := range e.policies {
		r := p.Evaluate(ctx, ec)
		trace = append(trace, PolicyOutcome{Policy: p.Name(), Result: r.String()})
		if r == Override {
			overridden = true
			continue
		}
		worst = Combine(worst, r)
	}
	if overridden {
		return Allow, trace
	}
	return worst, trace
}
