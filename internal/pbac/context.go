// Package pbac implements Warden's policy-based access control layer. PBAC
// runs after RBAC has granted a permission and evaluates runtime context
// (time, geography, target sensitivity, integration health, operator
// posture, etc.) to decide whether to allow, deny, or escalate to approval.
package pbac

import (
	"net"
	"time"
)

// EvalContext is the snapshot of runtime context passed to every policy.
// The caller (the action dispatch middleware) is responsible for populating
// these fields from request state, the operator record, the target identity,
// the integration health table, and the configured OnCallResolver.
//
// Fields are zero-valued when unavailable; policies must tolerate that
// (e.g. an empty GeoRegion means "IP geo failed, do not deny on geography").
type EvalContext struct {
	// Now is the wall-clock time at which the policy stack is evaluated.
	Now time.Time

	// SourceIP is the client IP of the inbound request (after any trusted
	// proxy is unwrapped). Nil if unavailable.
	SourceIP net.IP

	// GeoRegion is a coarse geographic label derived from SourceIP, e.g.
	// "US-CA" or "DE". Empty if geolocation failed or is disabled.
	GeoRegion string

	// SessionAge is the duration since the current session was created.
	SessionAge time.Duration

	// SessionCount is the number of active (non-expired) sessions for the
	// acting operator at evaluation time.
	SessionCount int

	// OperatorTenure is the time since the acting operator's user row was
	// created. Used by the new_operator_probation policy.
	OperatorTenure time.Duration

	// OnCall is the result of OnCallResolver.IsOnCall for the acting
	// operator. When ON_CALL_PROVIDER=none, this is always true.
	OnCall bool

	// TargetEmail is the target identity of the requested action. Empty for
	// actions that have no per-identity target (e.g. a bulk export).
	TargetEmail string

	// TargetIsVIP is true if TargetEmail is present in the vip_identities
	// table. Computed eagerly so policies can short-circuit on it.
	TargetIsVIP bool

	// TargetIsSelf is true if the target email equals the acting operator's
	// email (case-insensitive). Used by self_targeting_block.
	TargetIsSelf bool

	// InstanceName is the human-readable name of the integration instance
	// the action runs against (e.g. "okta-prod"). Empty if not applicable.
	InstanceName string

	// IntegrationHealthy reflects integration_instances.last_health_ok for
	// the target instance. True is the safe default when no health check
	// has yet run.
	IntegrationHealthy bool

	// ActionKey is the plugin action key being attempted, e.g.
	// "okta:deactivate_user" or "hold:release".
	ActionKey string

	// Destructive mirrors the plugin's ActionDefinition.Destructive flag.
	// Several policies are no-ops for non-destructive actions.
	Destructive bool

	// ActiveIncident is true if there is a currently-open declared incident
	// window covering this evaluation. Honored by incident_window_expansion
	// to short-circuit the policy stack to Allow.
	ActiveIncident bool

	// BulkCount is the number of times the acting operator has executed
	// ActionKey within the rolling bulk-detection window. Used by
	// bulk_action_threshold.
	BulkCount int

	// ActiveHoldCount is the number of currently-active legal holds that
	// have TargetEmail listed as a custodian. Used by legal_hold_conflict
	// to block release of one hold when the custodian is still on others.
	ActiveHoldCount int

	// LastBreakGlass is the time of the acting operator's most recent
	// break-glass invocation, if any. Nil if they have never used it. Used
	// by breakglass_cooldown.
	LastBreakGlass *time.Time
}
