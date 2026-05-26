package pbac

import (
	"context"
	"testing"
)

// staticPolicy returns a fixed Result regardless of context. Used to drive
// the engine's combine rules in unit tests.
type staticPolicy struct {
	name   string
	result Result
}

func (p staticPolicy) Name() string                                 { return p.name }
func (p staticPolicy) Evaluate(_ context.Context, _ EvalContext) Result { return p.result }

func TestCombine(t *testing.T) {
	cases := []struct {
		a, b Result
		want Result
	}{
		{Allow, Allow, Allow},
		{Allow, RequireApproval, RequireApproval},
		{Allow, Deny, Deny},
		{RequireApproval, Deny, Deny},
		{RequireApproval, RequireApproval, RequireApproval},
		{Deny, Deny, Deny},
		{Override, Deny, Override},
		{Override, Allow, Override},
		{Deny, Override, Override},
	}
	for _, c := range cases {
		got := Combine(c.a, c.b)
		if got != c.want {
			t.Errorf("Combine(%v,%v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestEngineEvaluate(t *testing.T) {
	e := NewEngine([]Policy{
		staticPolicy{"a", Allow},
		staticPolicy{"b", RequireApproval},
		staticPolicy{"c", Deny},
	})
	if got := e.Evaluate(context.Background(), EvalContext{}); got != Deny {
		t.Errorf("expected Deny, got %v", got)
	}

	e = NewEngine([]Policy{
		staticPolicy{"a", Allow},
		staticPolicy{"b", RequireApproval},
	})
	if got := e.Evaluate(context.Background(), EvalContext{}); got != RequireApproval {
		t.Errorf("expected RequireApproval, got %v", got)
	}

	// Override beats Deny.
	e = NewEngine([]Policy{
		staticPolicy{"a", Deny},
		staticPolicy{"b", Override},
	})
	if got := e.Evaluate(context.Background(), EvalContext{}); got != Allow {
		t.Errorf("expected Override→Allow, got %v", got)
	}

	// Empty engine allows.
	e = NewEngine(nil)
	if got := e.Evaluate(context.Background(), EvalContext{}); got != Allow {
		t.Errorf("empty engine should allow, got %v", got)
	}
}

func TestEvaluateWithTrace(t *testing.T) {
	e := NewEngine([]Policy{
		staticPolicy{"first", Allow},
		staticPolicy{"second", Deny},
	})
	result, trace := e.EvaluateWithTrace(context.Background(), EvalContext{})
	if result != Deny {
		t.Errorf("expected Deny, got %v", result)
	}
	if len(trace) != 2 {
		t.Fatalf("expected 2 trace entries, got %d", len(trace))
	}
	if trace[0].Policy != "first" || trace[0].Result != "allow" {
		t.Errorf("trace[0] = %+v", trace[0])
	}
	if trace[1].Policy != "second" || trace[1].Result != "deny" {
		t.Errorf("trace[1] = %+v", trace[1])
	}
}
