package policies

import (
	"context"
	"net"
	"testing"
	"time"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

func ctxAt(hour int) pbactypes.EvalContext {
	return pbactypes.EvalContext{Now: time.Date(2026, 1, 5, hour, 0, 0, 0, time.UTC)}
}

func TestTimeOfDay(t *testing.T) {
	p := TimeOfDay{Config: TimeOfDayConfig{StartHour: 9, EndHour: 18}}
	if got := p.Evaluate(context.Background(), ctxAt(10)); got != pbactypes.Allow {
		t.Errorf("inside window expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), ctxAt(20)); got != pbactypes.Deny {
		t.Errorf("outside window expected Deny, got %v", got)
	}
	// Wrap-around window 22..6.
	p = TimeOfDay{Config: TimeOfDayConfig{StartHour: 22, EndHour: 6}}
	if got := p.Evaluate(context.Background(), ctxAt(2)); got != pbactypes.Allow {
		t.Errorf("wrap-around early hours expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), ctxAt(12)); got != pbactypes.Deny {
		t.Errorf("midday with wrap window expected Deny, got %v", got)
	}
	// Destructive-only gate.
	p = TimeOfDay{Config: TimeOfDayConfig{StartHour: 9, EndHour: 18, ApplyToDestructiveOnly: true}}
	ec := ctxAt(22)
	ec.Destructive = false
	if got := p.Evaluate(context.Background(), ec); got != pbactypes.Allow {
		t.Errorf("non-destructive should bypass, got %v", got)
	}
	ec.Destructive = true
	if got := p.Evaluate(context.Background(), ec); got != pbactypes.Deny {
		t.Errorf("destructive outside window should Deny, got %v", got)
	}
}

func TestDayOfWeek(t *testing.T) {
	p := DayOfWeek{Config: DayOfWeekConfig{AllowedDays: []int{1, 2, 3, 4, 5}}}
	// 2026-01-05 is a Monday → allowed.
	ec := pbactypes.EvalContext{Now: time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)}
	if got := p.Evaluate(context.Background(), ec); got != pbactypes.Allow {
		t.Errorf("Monday expected Allow, got %v", got)
	}
	// 2026-01-03 is a Saturday → denied.
	ec.Now = time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	if got := p.Evaluate(context.Background(), ec); got != pbactypes.Deny {
		t.Errorf("Saturday expected Deny, got %v", got)
	}
}

func TestChangeFreezeWindow(t *testing.T) {
	w := FreezeWindow{
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
	}
	p := ChangeFreezeWindow{Config: ChangeFreezeWindowConfig{Windows: []FreezeWindow{w}}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Now: time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)}); got != pbactypes.Deny {
		t.Errorf("inside freeze expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Now: time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)}); got != pbactypes.Allow {
		t.Errorf("outside freeze expected Allow, got %v", got)
	}
}

func TestSourceIP(t *testing.T) {
	p := SourceIP{Config: SourceIPConfig{AllowedCIDRs: []string{"10.0.0.0/8"}}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{SourceIP: net.ParseIP("10.5.5.5")}); got != pbactypes.Allow {
		t.Errorf("IP in CIDR expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{SourceIP: net.ParseIP("8.8.8.8")}); got != pbactypes.Deny {
		t.Errorf("IP outside CIDR expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{SourceIP: nil}); got != pbactypes.Deny {
		t.Errorf("nil IP expected Deny by default, got %v", got)
	}
}

func TestGeographicAnomaly(t *testing.T) {
	p := GeographicAnomaly{Config: GeographicAnomalyConfig{AllowedRegions: []string{"US-CA"}}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{GeoRegion: "US-CA"}); got != pbactypes.Allow {
		t.Errorf("allowed region expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{GeoRegion: "RU-MOW"}); got != pbactypes.Deny {
		t.Errorf("disallowed region expected Deny, got %v", got)
	}
	// Empty GeoRegion (geolocation unavailable) is a no-op.
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{GeoRegion: ""}); got != pbactypes.Allow {
		t.Errorf("empty region expected Allow, got %v", got)
	}
}

func TestStepUpMFA(t *testing.T) {
	p := StepUpMFA{Config: StepUpMFAConfig{MaxSessionAgeMinutes: 30}}
	// Non-destructive: always allow.
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: false, SessionAge: time.Hour}); got != pbactypes.Allow {
		t.Errorf("non-destructive expected Allow, got %v", got)
	}
	// Destructive + fresh session: allow.
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, SessionAge: 10 * time.Minute}); got != pbactypes.Allow {
		t.Errorf("fresh session expected Allow, got %v", got)
	}
	// Destructive + stale session: require approval.
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, SessionAge: 2 * time.Hour}); got != pbactypes.RequireApproval {
		t.Errorf("stale session destructive expected RequireApproval, got %v", got)
	}
}

func TestConcurrentSessionLimit(t *testing.T) {
	p := ConcurrentSessionLimit{Config: ConcurrentSessionLimitConfig{MaxSessions: 3}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{SessionCount: 2}); got != pbactypes.Allow {
		t.Errorf("under limit expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{SessionCount: 5}); got != pbactypes.Deny {
		t.Errorf("over limit expected Deny, got %v", got)
	}
}

func TestVIPProtection(t *testing.T) {
	if got := (VIPProtection{}).Evaluate(context.Background(), pbactypes.EvalContext{TargetIsVIP: true}); got != pbactypes.RequireApproval {
		t.Errorf("VIP target expected RequireApproval, got %v", got)
	}
	if got := (VIPProtection{}).Evaluate(context.Background(), pbactypes.EvalContext{TargetIsVIP: false}); got != pbactypes.Allow {
		t.Errorf("non-VIP expected Allow, got %v", got)
	}
}

func TestSelfTargetingBlock(t *testing.T) {
	if got := (SelfTargetingBlock{}).Evaluate(context.Background(), pbactypes.EvalContext{TargetIsSelf: true}); got != pbactypes.Deny {
		t.Errorf("self-target expected Deny, got %v", got)
	}
	if got := (SelfTargetingBlock{}).Evaluate(context.Background(), pbactypes.EvalContext{TargetIsSelf: false}); got != pbactypes.Allow {
		t.Errorf("non-self expected Allow, got %v", got)
	}
}

func TestBulkActionThreshold(t *testing.T) {
	p := BulkActionThreshold{Config: BulkActionThresholdConfig{MaxCount: 10, WindowMinutes: 5}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{BulkCount: 5}); got != pbactypes.Allow {
		t.Errorf("under count expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{BulkCount: 50}); got != pbactypes.Deny {
		t.Errorf("over count expected Deny, got %v", got)
	}
}

func TestLegalHoldConflict(t *testing.T) {
	p := LegalHoldConflict{}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: HoldReleaseActionKey, ActiveHoldCount: 2}); got != pbactypes.Deny {
		t.Errorf("release with 2 holds expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: HoldReleaseActionKey, ActiveHoldCount: 1}); got != pbactypes.Allow {
		t.Errorf("release with 1 hold expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: "okta:deactivate", ActiveHoldCount: 5}); got != pbactypes.Allow {
		t.Errorf("non-release action expected Allow, got %v", got)
	}
}

func TestProductionInstanceGate(t *testing.T) {
	p := ProductionInstanceGate{Config: ProductionInstanceGateConfig{ProdPattern: "*-prod"}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, InstanceName: "okta-prod"}); got != pbactypes.RequireApproval {
		t.Errorf("prod instance destructive expected RequireApproval, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, InstanceName: "okta-staging"}); got != pbactypes.Allow {
		t.Errorf("staging instance expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: false, InstanceName: "okta-prod"}); got != pbactypes.Allow {
		t.Errorf("non-destructive against prod expected Allow, got %v", got)
	}
}

func TestIntegrationHealthCheck(t *testing.T) {
	if got := (IntegrationHealthCheck{}).Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, IntegrationHealthy: false}); got != pbactypes.Deny {
		t.Errorf("unhealthy destructive expected Deny, got %v", got)
	}
	if got := (IntegrationHealthCheck{}).Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, IntegrationHealthy: true}); got != pbactypes.Allow {
		t.Errorf("healthy destructive expected Allow, got %v", got)
	}
	if got := (IntegrationHealthCheck{}).Evaluate(context.Background(), pbactypes.EvalContext{Destructive: false, IntegrationHealthy: false}); got != pbactypes.Allow {
		t.Errorf("unhealthy non-destructive expected Allow, got %v", got)
	}
}

func TestNewOperatorProbation(t *testing.T) {
	p := NewOperatorProbation{Config: NewOperatorProbationConfig{ProbationDays: 30}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, OperatorTenure: 10 * 24 * time.Hour}); got != pbactypes.Deny {
		t.Errorf("new operator destructive expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: true, OperatorTenure: 60 * 24 * time.Hour}); got != pbactypes.Allow {
		t.Errorf("tenured operator expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{Destructive: false, OperatorTenure: 1 * time.Hour}); got != pbactypes.Allow {
		t.Errorf("non-destructive should bypass, got %v", got)
	}
}

func TestOnCallVerification(t *testing.T) {
	p := OnCallVerification{Config: OnCallVerificationConfig{RequiredFor: []string{"*:wipe", "*:mass_*"}}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: "jamf:wipe", OnCall: false}); got != pbactypes.Deny {
		t.Errorf("required pattern off-call expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: "jamf:wipe", OnCall: true}); got != pbactypes.Allow {
		t.Errorf("on-call expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: "okta:lookup", OnCall: false}); got != pbactypes.Allow {
		t.Errorf("non-required action expected Allow, got %v", got)
	}
}

func TestBreakGlassCooldown(t *testing.T) {
	now := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Minute)
	older := now.Add(-25 * time.Hour)

	p := BreakGlassCooldown{Config: BreakGlassCooldownConfig{CooldownHours: 24}}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: BreakGlassActionKey, Now: now, LastBreakGlass: &recent}); got != pbactypes.Deny {
		t.Errorf("within cooldown expected Deny, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: BreakGlassActionKey, Now: now, LastBreakGlass: &older}); got != pbactypes.Allow {
		t.Errorf("past cooldown expected Allow, got %v", got)
	}
	if got := p.Evaluate(context.Background(), pbactypes.EvalContext{ActionKey: "okta:lookup", Now: now, LastBreakGlass: &recent}); got != pbactypes.Allow {
		t.Errorf("non-breakglass action expected Allow, got %v", got)
	}
}

func TestIncidentWindowExpansion(t *testing.T) {
	if got := (IncidentWindowExpansion{}).Evaluate(context.Background(), pbactypes.EvalContext{ActiveIncident: true}); got != pbactypes.Override {
		t.Errorf("active incident expected Override, got %v", got)
	}
	if got := (IncidentWindowExpansion{}).Evaluate(context.Background(), pbactypes.EvalContext{ActiveIncident: false}); got != pbactypes.Allow {
		t.Errorf("no incident expected Allow, got %v", got)
	}
}
