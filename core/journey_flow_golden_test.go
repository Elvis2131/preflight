package core_test

// PC-125/PC-149: proves ComputeJourneyFlow against the real golden AWS bundle.
// golden/aws declares no NACL resources, so every subnet is associated with its
// VPC's default NACL (AWS VPC User Guide, "Control subnet traffic with network access
// control lists") and, with nothing declared, that default is the documented
// allow-all, ASSUMED unmodified (PC-149). Hand-verified hop by hop: internet -> ALB
// (tcp/443; the ALB SG allows 0.0.0.0/0 on 443) is Allowed. ALB -> EKS then blocks at
// the SECURITY GROUP step, for a real reason in the design rather than the engine: the
// journey declares one protocol/port (tcp/443) for every hop, while golden's workload SG
// only admits tcp/8080 from the ALB SG (security.tf workload_from_alb). A single-port
// journey cannot express a path whose hops use different ports (recorded on PC-149).

import (
	"strings"
	"testing"

	"preflight/core"
)

func TestGoldenWorkload_CheckoutJourney_StructuralFlow_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)

	var checkout *core.DeclaredJourney
	for i := range workload.Journeys {
		if workload.Journeys[i].ID == "checkout" {
			checkout = &workload.Journeys[i]
		}
	}
	if checkout == nil {
		t.Fatal("expected a \"checkout\" journey in golden/workload.yaml")
	}

	flow := core.ComputeJourneyFlow(ir, *checkout, nil)
	if flow.Flows {
		t.Fatalf("got Flows=true, want false — the single declared port (tcp/443) is not what the ALB->workload SG rule admits (tcp/8080): %+v", flow)
	}
	if len(flow.Hops) != 2 {
		t.Fatalf("got %d hops, want exactly 2 (internet->ALB allowed, ALB->EKS blocked)", len(flow.Hops))
	}
	if !flow.Hops[0].Allowed {
		t.Errorf("internet -> aws_lb.payments must be Allowed under the (assumed) default NACL: %+v", flow.Hops[0])
	}
	if flow.BlockedAt != "aws_eks_cluster.payments" {
		t.Errorf("BlockedAt = %q, want aws_eks_cluster.payments", flow.BlockedAt)
	}
	if !strings.Contains(flow.BlockedReason, "sg_dest_ingress") {
		t.Errorf("BlockedReason = %q, want it to name the sg_dest_ingress step", flow.BlockedReason)
	}
}
