package core_test

// PC-125: proves ComputeJourneyFlow against the real golden AWS bundle. Golden/aws
// has zero NACL resources (PC-113's own documented, pre-existing gap — "golden
// bundles have zero NACL resources... entirely synthetic-fixture coverage") — so a
// real cross-subnet request in this exact bundle is honestly BLOCKED at NACL
// resolution, not a fabricated success. Hand-verified against that real, true fact,
// not a wished-for happy path.

import (
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
		t.Fatalf("got Flows=true, want false — golden/aws has zero NACL resources (PC-113), so the internet->ALB hop must honestly block at NACL resolution: %+v", flow)
	}
	if len(flow.Hops) != 1 {
		t.Fatalf("got %d hops, want exactly 1 (blocked on the very first hop)", len(flow.Hops))
	}
	if flow.BlockedAt != "aws_lb.payments" {
		t.Errorf("BlockedAt = %q, want aws_lb.payments", flow.BlockedAt)
	}
	if flow.BlockedReason == "" {
		t.Error("BlockedReason is empty, want a real citation of the NACL gap")
	}
}
