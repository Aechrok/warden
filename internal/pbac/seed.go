package pbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type policySpec struct {
	name       string
	policyType string
	enabled    bool
	config     string
}

// allSpecs lists every known PBAC policy with its seed defaults.
// The four enabled-by-default policies match DefaultPolicies().
// All others are seeded disabled so admins can opt in.
var allSpecs = []policySpec{
	{name: "vip_protection", policyType: "vip_protection", enabled: true, config: `{}`},
	{name: "production_instance_gate", policyType: "production_instance_gate", enabled: true, config: `{"prod_pattern":"*-prod"}`},
	{name: "change_freeze_window", policyType: "change_freeze_window", enabled: true, config: `{"applies_to_destructive_only":true}`},
	{name: "step_up_mfa", policyType: "step_up_mfa", enabled: true, config: `{"max_session_age_minutes":60}`},
	{name: "time_of_day", policyType: "time_of_day", enabled: false, config: `{}`},
	{name: "day_of_week", policyType: "day_of_week", enabled: false, config: `{}`},
	{name: "source_ip", policyType: "source_ip", enabled: false, config: `{}`},
	{name: "geographic_anomaly", policyType: "geographic_anomaly", enabled: false, config: `{}`},
	{name: "concurrent_session_limit", policyType: "concurrent_session_limit", enabled: false, config: `{}`},
	{name: "self_targeting_block", policyType: "self_targeting_block", enabled: false, config: `{}`},
	{name: "bulk_action_threshold", policyType: "bulk_action_threshold", enabled: false, config: `{}`},
	{name: "legal_hold_conflict", policyType: "legal_hold_conflict", enabled: false, config: `{}`},
	{name: "integration_health_check", policyType: "integration_health_check", enabled: false, config: `{}`},
	{name: "new_operator_probation", policyType: "new_operator_probation", enabled: false, config: `{}`},
	{name: "on_call_verification", policyType: "on_call_verification", enabled: false, config: `{}`},
	{name: "breakglass_cooldown", policyType: "breakglass_cooldown", enabled: false, config: `{}`},
	{name: "incident_window_expansion", policyType: "incident_window_expansion", enabled: false, config: `{}`},
}

// SeedPolicies inserts all known PBAC policies with their defaults.
// Existing rows are left untouched (ON CONFLICT DO NOTHING), so this is safe
// to call on every startup.
func SeedPolicies(ctx context.Context, pool *pgxpool.Pool) error {
	for _, s := range allSpecs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO pbac_policies (name, policy_type, is_enabled, config)
			VALUES ($1, $2, $3, $4::jsonb)
			ON CONFLICT (name) DO NOTHING
		`, s.name, s.policyType, s.enabled, s.config); err != nil {
			return fmt.Errorf("pbac: seed %q: %w", s.name, err)
		}
	}
	return nil
}
