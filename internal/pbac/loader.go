package pbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/aechrok/warden/internal/pbac/policies"
)

// LoadFromDB reads every enabled row from pbac_policies and constructs the
// corresponding Policy implementation. Rows whose policy_type is unknown are
// skipped with no error so that a forward-rolled config (e.g. a policy type
// added in a newer release) does not crash the server. The caller may use
// the returned slice directly with NewEngine.
func LoadFromDB(ctx context.Context, pool *pgxpool.Pool) ([]Policy, error) {
	if pool == nil {
		return nil, errors.New("pbac: load: nil pool")
	}
	rows, err := pool.Query(ctx, `
		SELECT name, policy_type, config
		FROM pbac_policies
		WHERE is_enabled = true
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("pbac: query policies: %w", err)
	}
	defer rows.Close()

	out := []Policy{}
	for rows.Next() {
		var (
			name       string
			policyType string
			configRaw  []byte
		)
		if err := rows.Scan(&name, &policyType, &configRaw); err != nil {
			return nil, fmt.Errorf("pbac: scan policy: %w", err)
		}
		p, err := buildPolicy(policyType, configRaw)
		if err != nil {
			return nil, fmt.Errorf("pbac: build policy %q (%s): %w", name, policyType, err)
		}
		if p == nil {
			continue
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pbac: rows: %w", err)
	}
	return out, nil
}

// buildPolicy maps a policy_type string + JSON config to a concrete Policy.
// Unknown types return (nil, nil) so the loader can ignore them.
func buildPolicy(policyType string, configRaw []byte) (Policy, error) {
	if len(configRaw) == 0 {
		configRaw = []byte("{}")
	}
	switch policyType {
	case "time_of_day":
		var c policies.TimeOfDayConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.TimeOfDay{Config: c}, nil
	case "day_of_week":
		var c policies.DayOfWeekConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.DayOfWeek{Config: c}, nil
	case "change_freeze_window":
		var c policies.ChangeFreezeWindowConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.ChangeFreezeWindow{Config: c}, nil
	case "source_ip":
		var c policies.SourceIPConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.SourceIP{Config: c}, nil
	case "geographic_anomaly":
		var c policies.GeographicAnomalyConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.GeographicAnomaly{Config: c}, nil
	case "step_up_mfa":
		var c policies.StepUpMFAConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.StepUpMFA{Config: c}, nil
	case "concurrent_session_limit":
		var c policies.ConcurrentSessionLimitConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.ConcurrentSessionLimit{Config: c}, nil
	case "vip_protection":
		return policies.VIPProtection{}, nil
	case "self_targeting_block":
		return policies.SelfTargetingBlock{}, nil
	case "bulk_action_threshold":
		var c policies.BulkActionThresholdConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.BulkActionThreshold{Config: c}, nil
	case "legal_hold_conflict":
		return policies.LegalHoldConflict{}, nil
	case "production_instance_gate":
		var c policies.ProductionInstanceGateConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.ProductionInstanceGate{Config: c}, nil
	case "integration_health_check":
		return policies.IntegrationHealthCheck{}, nil
	case "new_operator_probation":
		var c policies.NewOperatorProbationConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.NewOperatorProbation{Config: c}, nil
	case "on_call_verification":
		var c policies.OnCallVerificationConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.OnCallVerification{Config: c}, nil
	case "breakglass_cooldown":
		var c policies.BreakGlassCooldownConfig
		if err := json.Unmarshal(configRaw, &c); err != nil {
			return nil, err
		}
		return policies.BreakGlassCooldown{Config: c}, nil
	case "incident_window_expansion":
		return policies.IncidentWindowExpansion{}, nil
	default:
		return nil, nil
	}
}

// DefaultPolicies returns the four must-have policies that ship enabled by
// default. The remaining 13 policies are available but disabled until an
// admin opts in.
//
//   - vip_protection — no config
//   - production_instance_gate — "*-prod" glob
//   - change_freeze_window — empty windows (effectively disabled until configured)
//   - step_up_mfa — 60-minute session age threshold for destructive actions
func DefaultPolicies() []Policy {
	return []Policy{
		policies.VIPProtection{},
		policies.ProductionInstanceGate{Config: policies.ProductionInstanceGateConfig{ProdPattern: "*-prod"}},
		policies.ChangeFreezeWindow{Config: policies.ChangeFreezeWindowConfig{AppliesToDestructiveOnly: true}},
		policies.StepUpMFA{Config: policies.StepUpMFAConfig{MaxSessionAgeMinutes: 60}},
	}
}
