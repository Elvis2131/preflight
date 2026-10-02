package core_test

// PC-125/PC-149/PC-151/PC-152: proves ComputeJourneyFlow against the real golden AWS
// bundle. golden/aws declares no NACLs and its data subnets have no explicit route table
// association, so (assumed unmodified, tagged so) every subnet is on its VPC's default NACL
// and the unassociated ones use the VPC's main route table. With each hop's real port
// declared (workload.yaml hop_ports: ALB 443, workload 8080, database 5432) the checkout
// journey flows end to end. Hand-verified hop by hop against security.tf: the ALB SG admits
// 0.0.0.0/0 on 443, the workload SG admits the ALB SG on 8080, and the database SG admits
// the workload SG on 5432. The settlement journey (EKS -> SQS) stays blocked: SQS is a
// regional endpoint with no subnet, so its route is genuinely not assessable.

import (
	"strings"
	"testing"

	"preflight/core"
)

func goldenJourney(t *testing.T, id string) core.DeclaredJourney {
	t.Helper()
	for _, j := range loadGoldenWorkload(t).Journeys {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("expected a %q journey in golden/workload.yaml", id)
	return core.DeclaredJourney{}
}

func TestGoldenWorkload_CheckoutJourney_StructuralFlow_HandVerified(t *testing.T) {
	flow := core.ComputeJourneyFlow(realGoldenIR(t), goldenJourney(t, "checkout"), nil)
	if !flow.Flows {
		t.Fatalf("the golden checkout journey must flow end to end with its declared per-hop ports: %+v", flow)
	}
	if len(flow.Hops) != 3 {
		t.Fatalf("got %d hops, want 3 (internet->ALB, ALB->EKS, EKS->RDS)", len(flow.Hops))
	}
	for _, h := range flow.Hops {
		if !h.Allowed {
			t.Errorf("hop %s -> %s must be allowed: %s", h.From, h.To, h.Reason)
		}
	}
}

func TestGoldenWorkload_SettlementJourney_StaysNotAssessable(t *testing.T) {
	flow := core.ComputeJourneyFlow(realGoldenIR(t), goldenJourney(t, "settlement"), nil)
	if flow.Flows || flow.BlockedAt != "aws_sqs_queue.settlement" || !strings.Contains(flow.BlockedReason, "route_selection") {
		t.Fatalf("EKS -> SQS must stay blocked at route_selection (a regional endpoint has no subnet): %+v", flow)
	}
}
