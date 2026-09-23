// This file is a real output-shape fix, found while starting PC-28: driving the
// agent-iteration harness off /assess's existing findings revealed that only ONE of
// the three findings core.BuildFindings produced was actually delta-visible (the RDS
// compliance check) — the zone-kill findings either didn't differ between the broken
// and clean golden bundles at all, or used a free-form Outcome value ComputeDelta
// cannot rank by design (PC-19's own stated scope boundary). Per PC-28's Card: "treat
// any agent failure to converge as a signal to fix the output shape, not the agent."
// This is that fix, not a workaround around it.
//
// NATGatewayRedundancyCheck answers a purely topological question — no AWS-behavior
// claim needing external doc verification, unlike PC-18's RDS check: does every
// declared public subnet retain independent NAT gateway coverage? This directly
// detects golden/aws-broken's defect 1 (network.tf: only one NAT gateway serves three
// subnets) as a real, ComplianceStatus-shaped, delta-rankable finding — something no
// existing finding did.
//
// Same package rule as everywhere else here: no dependency on preflight/core.
package analyse

import (
	"fmt"
	"sort"
)

// NATGatewayRedundancyCheck takes, for each declared public subnet, how many NAT
// gateways are reachable from it (via ContainmentBlastRadius, counted by the caller —
// see core/findings_builder.go). satisfied only if every subnet has at least one;
// otherwise unsatisfied, naming exactly which subnet(s) lack coverage.
func NATGatewayRedundancyCheck(natGatewayCountBySubnet map[string]int) (status string, rationale string) {
	if len(natGatewayCountBySubnet) == 0 {
		return "not_assessable", "no public subnets were found to check NAT gateway coverage against"
	}

	var uncovered []string
	for subnet, count := range natGatewayCountBySubnet {
		if count == 0 {
			uncovered = append(uncovered, subnet)
		}
	}
	sort.Strings(uncovered) // determinism (I1)

	if len(uncovered) == 0 {
		return "satisfied", fmt.Sprintf("all %d public subnet(s) have independent NAT gateway coverage", len(natGatewayCountBySubnet))
	}
	return "unsatisfied", fmt.Sprintf("subnet(s) %v have no NAT gateway of their own — losing their AZ's gateway (or the single shared one) leaves them with no egress path", uncovered)
}
