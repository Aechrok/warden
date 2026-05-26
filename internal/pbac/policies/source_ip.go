package policies

import (
	"context"
	"net"

	pbactypes "github.com/aechrok/warden/internal/pbac/types"
)

// SourceIPConfig configures the SourceIP policy. AllowedCIDRs is a list of
// network ranges (IPv4 or IPv6, CIDR notation). RequireApprovalOnViolation
// changes the result from Deny → RequireApproval, useful for environments
// that want to track but not block off-network access.
type SourceIPConfig struct {
	AllowedCIDRs               []string `json:"allowed_cidrs"`
	RequireApprovalOnViolation bool     `json:"require_approval_on_violation"`
}

// SourceIP denies requests originating outside an allowlisted IP range.
type SourceIP struct {
	Config SourceIPConfig

	// nets is the parsed form of Config.AllowedCIDRs. Populated lazily on
	// first Evaluate so an instance constructed with a stale config still
	// works after a hot reload.
	nets []*net.IPNet
}

// Name implements pbac.Policy.
func (SourceIP) Name() string { return "source_ip" }

// Evaluate implements pbac.Policy.
func (p SourceIP) Evaluate(_ context.Context, ec pbactypes.EvalContext) pbactypes.Result {
	if ec.SourceIP == nil {
		// No client IP available — fail closed only when violation is
		// configured as Deny; we cannot prove the IP is in the allowlist.
		if p.Config.RequireApprovalOnViolation {
			return pbactypes.RequireApproval
		}
		return pbactypes.Deny
	}
	nets := parseCIDRs(p.Config.AllowedCIDRs)
	for _, n := range nets {
		if n.Contains(ec.SourceIP) {
			return pbactypes.Allow
		}
	}
	if p.Config.RequireApprovalOnViolation {
		return pbactypes.RequireApproval
	}
	return pbactypes.Deny
}

func parseCIDRs(in []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(in))
	for _, raw := range in {
		_, n, err := net.ParseCIDR(raw)
		if err != nil || n == nil {
			continue
		}
		out = append(out, n)
	}
	return out
}
