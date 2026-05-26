package policies

import (
	"context"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// IncidentWindowExpansion is the policy that expands operator permissions
// during a declared incident. When ec.ActiveIncident is true, it returns
// pbac.Override which short-circuits the engine to Allow regardless of any
// other policy's Deny / RequireApproval. The incident itself is declared
// out-of-band (by the on-call admin via an internal API) and recorded as
// an event in the audit log; every expanded action is similarly recorded.
type IncidentWindowExpansion struct{}

// Name implements pbac.Policy.
func (IncidentWindowExpansion) Name() string { return "incident_window_expansion" }

// Evaluate implements pbac.Policy.
func (IncidentWindowExpansion) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if ec.ActiveIncident {
		return pbactypes.Override
	}
	return pbactypes.Allow
}
