// Package policies contains the concrete implementations of the 17 PBAC
// policies that ship with Warden. Each policy is a stateless value-receiver
// struct that returns one of pbac.Allow, pbac.RequireApproval, pbac.Deny, or
// (for incident_window_expansion only) pbac.Override.
//
// Policy names match the policy_type column of the pbac_policies table; the
// loader uses these names to map a stored row to a constructor.
package policies

// Names lists every policy ID known to the loader, useful for the admin UI
// "add policy" picker and for config validators.
var Names = []string{
	"time_of_day",
	"day_of_week",
	"change_freeze_window",
	"source_ip",
	"geographic_anomaly",
	"step_up_mfa",
	"concurrent_session_limit",
	"vip_protection",
	"self_targeting_block",
	"bulk_action_threshold",
	"legal_hold_conflict",
	"production_instance_gate",
	"integration_health_check",
	"new_operator_probation",
	"on_call_verification",
	"breakglass_cooldown",
	"incident_window_expansion",
}
