package core_test

// PC-155: outbound journeys (a journey may end at the internet).

import (
	"strings"
	"testing"

	"preflight/core"
)

func TestOutboundJourney_Golden_FlowsViaNAT(t *testing.T) {
	ir := realGoldenIR(t)
	flow := core.ComputeJourneyFlow(ir, goldenJourney(t, "payment-rail"), nil)
	if !flow.Flows {
		t.Fatalf("payment-rail blocked: %s: %s", flow.BlockedAt, flow.BlockedReason)
	}
	last := flow.Hops[len(flow.Hops)-1]
	if last.To != core.JourneyInternetSentinel || !last.Allowed {
		t.Errorf("last hop = %+v, want allowed to internet", last)
	}
}

func TestOutboundTrace_Golden_StepsInOrder(t *testing.T) {
	tr := core.BuildOutboundTrace(realGoldenIR(t), "aws_eks_cluster.payments", "tcp", 443)
	var names []string
	for _, s := range tr.Steps {
		names = append(names, s.Step)
	}
	t.Logf("allowed=%v steps=%v concise=%s", tr.Allowed, names, tr.Concise)
	for _, s := range tr.Steps {
		t.Logf("  %s %s %s: %s", s.Step, s.Component, s.Decision, s.Reason)
	}
	if !tr.Allowed {
		t.Fatalf("blocked: %s", tr.Concise)
	}
}

// Losing the NAT gateway breaks the journey and the trace names the route step.
func TestOutboundJourney_Golden_NATLossBreaksAtRouteStep(t *testing.T) {
	ir := realGoldenIR(t)
	tr0 := core.BuildOutboundTrace(ir, "aws_eks_cluster.payments", "tcp", 443)
	if !tr0.Allowed {
		t.Fatalf("baseline blocked: %s", tr0.Concise)
	}
	// The cluster spans three private subnets; its first resolvable subnet's NAT is the one
	// the trace used. Lose every NAT gateway in turn: exactly the one serving that subnet
	// must break the journey.
	broke := 0
	for _, id := range []string{"aws_nat_gateway.nat_a", "aws_nat_gateway.nat_b", "aws_nat_gateway.nat_c"} {
		tr := core.BuildOutboundTrace(core.WithNATGatewayLost(ir, id), "aws_eks_cluster.payments", "tcp", 443)
		if !tr.Allowed {
			broke++
			if !strings.Contains(tr.Concise, "route_selection") || !strings.Contains(tr.Concise, id) {
				t.Errorf("concise = %q, want it to name route_selection and %s", tr.Concise, id)
			}
		}
	}
	if broke != 1 {
		t.Errorf("%d NAT losses broke the journey, want exactly 1 (the NAT serving the resolved subnet)", broke)
	}
}

func TestOutboundTrace_SGEgressBlocked(t *testing.T) {
	ir := realGoldenIR(t)
	tr := core.BuildOutboundTrace(ir, "aws_eks_cluster.payments", "tcp", 22)
	if tr.Allowed {
		t.Fatal("port 22 outbound allowed, want blocked: the workload SG only permits 443 egress")
	}
	if !strings.Contains(tr.Concise, "sg_source_egress") && !strings.Contains(tr.Concise, "nacl_source_egress") {
		t.Errorf("concise = %q, want a NACL or SG egress step", tr.Concise)
	}
}

func TestOutboundTrace_UnknownSource_Denied(t *testing.T) {
	tr := core.BuildOutboundTrace(realGoldenIR(t), "aws_nope.x", "tcp", 443)
	if tr.Allowed || !strings.Contains(tr.Concise, "resolve_source") {
		t.Errorf("got %q, want blocked at resolve_source", tr.Concise)
	}
}

// Journey-level NAT loss: the flow names the route step, via the real fault mutation.
func TestOutboundJourney_Golden_NATLossBlocksJourney(t *testing.T) {
	ir := realGoldenIR(t)
	j := goldenJourney(t, "payment-rail")
	blocked := 0
	for _, id := range []string{"aws_nat_gateway.nat_a", "aws_nat_gateway.nat_b", "aws_nat_gateway.nat_c"} {
		flow := core.ComputeJourneyFlow(core.WithNATGatewayLost(ir, id), j, nil)
		if !flow.Flows {
			blocked++
			if flow.BlockedAt != core.JourneyInternetSentinel || !strings.Contains(flow.BlockedReason, "route_selection") {
				t.Errorf("%s: blocked at %q reason %q, want internet / route_selection", id, flow.BlockedAt, flow.BlockedReason)
			}
		}
	}
	if blocked != 1 {
		t.Errorf("%d NAT losses blocked the journey, want 1", blocked)
	}
}

// Hand-verified on golden: 60 rps x (4096 request + 2048 response bytes) x 2,628,000 s
// / 1024^3. Request GB = 157,680,000 / 262,144 = 601.50146 GB; total = 1.5x that =
// 902.25220 GB. Internet egress bills only the bytes leaving AWS (the 4096-byte requests):
// 601.50146 x $0.09 (first tier) = $54.1351. NAT data processing bills both directions:
// 902.25220 x $0.048 = $43.3081.
func TestGoldenUsageCost_PaymentRail_HandVerified(t *testing.T) {
	var egress, nat *core.UsageBasedCostEntry
	res := core.ComputeUsageBasedCost(realGoldenIR(t), loadGoldenWorkload(t), usageCostPriceTable(), nil)
	for i := range res {
		r := &res[i]
		if r.JourneyID != "payment-rail" || r.HopTo != core.JourneyInternetSentinel {
			continue
		}
		switch r.Kind {
		case core.UsageCostInternetEgress:
			egress = r
		case core.UsageCostNATDataProcessing:
			nat = r
		}
	}
	if egress == nil || nat == nil {
		t.Fatalf("missing entries: egress=%v nat=%v in %+v", egress, nat, res)
	}
	if egress.Decision != core.CostPriced || egress.MonthlyAmount < 54.12 || egress.MonthlyAmount > 54.15 {
		t.Errorf("egress = %v $%.4f, want priced $54.1351", egress.Decision, egress.MonthlyAmount)
	}
	if nat.Decision != core.CostPriced || nat.MonthlyAmount < 43.30 || nat.MonthlyAmount > 43.32 {
		t.Errorf("nat = %v $%.4f, want priced $43.3081", nat.Decision, nat.MonthlyAmount)
	}
}
