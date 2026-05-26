// Package pbac implements Warden's policy-based access control layer. PBAC
// runs after RBAC has granted a permission and evaluates runtime context
// (time, geography, target sensitivity, integration health, operator
// posture, etc.) to decide whether to allow, deny, or escalate to approval.
package pbac

import "github.com/aechrok/warden/internal/pbac/types"

// EvalContext is the snapshot of runtime context passed to every policy.
// The caller (the action dispatch middleware) is responsible for populating
// these fields from request state, the operator record, the target identity,
// the integration health table, and the configured OnCallResolver.
//
// Fields are zero-valued when unavailable; policies must tolerate that
// (e.g. an empty GeoRegion means "IP geo failed, do not deny on geography").
type EvalContext = types.EvalContext
