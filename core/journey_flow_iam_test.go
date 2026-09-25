package core_test

// PC-135's first acceptance criterion: "Trace for a call blocked only by IAM shows
// network steps passing and the IAM step failing with the deciding statement —
// tested on a fixture." Exercised both directly against BuildTraceWithIAM (the
// trace-level guarantee) and through ComputeJourneyFlow's own final-hop IAM check
// (the journey-level integration PC-135 also requires).

import (
	"strings"
	"testing"

	"preflight/core"
)

func TestBuildTraceWithIAM_NetworkPassesIAMFails_CitesDecidingStatement(t *testing.T) {
	ir := buildFlowTestIR(true, true) // network fully allows app -> db
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "OnlyRead", Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	ir.Nodes = append(ir.Nodes, role)

	trace := core.BuildTraceWithIAM(ir, "app", "db", "role", "dynamodb:DeleteItem", "*", "", "tcp", 5432)

	if trace.Allowed {
		t.Fatal("got Allowed=true, want false — the role's policy only grants GetItem, not DeleteItem")
	}
	var networkSteps, iamStep *core.TraceStep
	for i := range trace.Steps {
		s := &trace.Steps[i]
		if s.Step == "sg_dest_ingress" {
			networkSteps = s
		}
		if s.Step == "iam_check" {
			iamStep = s
		}
	}
	if networkSteps == nil || networkSteps.Decision != core.TraceAllow {
		t.Fatalf("network steps did not pass: %+v", trace.Steps)
	}
	if iamStep == nil {
		t.Fatal("expected an iam_check step")
	}
	if iamStep.Decision != core.TraceDeny {
		t.Fatalf("iam_check decision = %q, want deny", iamStep.Decision)
	}
	if !strings.Contains(trace.Concise, "iam_check") {
		t.Errorf("Concise = %q, want it to name the iam_check step as where this request is blocked", trace.Concise)
	}
}

func TestBuildTraceWithIAM_NetworkAndIAMBothAllow(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	ir.Nodes = append(ir.Nodes, role)

	trace := core.BuildTraceWithIAM(ir, "app", "db", "role", "dynamodb:GetItem", "*", "", "tcp", 5432)
	if !trace.Allowed {
		t.Fatalf("got Allowed=false, want true: %+v", trace.Steps)
	}
}

func TestBuildTraceWithIAM_NetworkBlocked_IAMStepNeverReached(t *testing.T) {
	ir := buildFlowTestIR(false, true) // SG denies
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	ir.Nodes = append(ir.Nodes, role)

	trace := core.BuildTraceWithIAM(ir, "app", "db", "role", "dynamodb:GetItem", "*", "", "tcp", 5432)
	if trace.Allowed {
		t.Fatal("got Allowed=true, want false — SG already denies this request")
	}
	var iamStep *core.TraceStep
	for i := range trace.Steps {
		if trace.Steps[i].Step == "iam_check" {
			iamStep = &trace.Steps[i]
		}
	}
	if iamStep == nil {
		t.Fatal("expected an (unreached) iam_check step to be recorded")
	}
	if iamStep.Reached {
		t.Error("iam_check.Reached = true, want false — a preceding network step already blocked this request")
	}
	if !strings.Contains(trace.Concise, "sg_dest_ingress") {
		t.Errorf("Concise = %q, want the network failure (sg_dest_ingress) cited, not IAM", trace.Concise)
	}
}

func TestComputeJourneyFlow_IAMCheck_FinalHop(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	ir.Nodes = append(ir.Nodes, role)

	allowedJourney := core.DeclaredJourney{
		ID: "j-allowed", Name: "j-allowed", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
		IAMCheck: &core.JourneyIAMCheck{PrincipalID: "role", Action: "dynamodb:GetItem", ResourceARN: "*"},
	}
	flow := core.ComputeJourneyFlow(ir, allowedJourney, nil)
	if !flow.Flows {
		t.Fatalf("got Flows=false, want true: %+v", flow)
	}

	deniedJourney := core.DeclaredJourney{
		ID: "j-denied", Name: "j-denied", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
		IAMCheck: &core.JourneyIAMCheck{PrincipalID: "role", Action: "dynamodb:DeleteItem", ResourceARN: "*"},
	}
	flow = core.ComputeJourneyFlow(ir, deniedJourney, nil)
	if flow.Flows {
		t.Fatal("got Flows=true, want false — the role's policy does not grant DeleteItem")
	}
	if flow.BlockedAt != "db" {
		t.Errorf("BlockedAt = %q, want %q", flow.BlockedAt, "db")
	}
	if !strings.Contains(flow.BlockedReason, "iam_check") {
		t.Errorf("BlockedReason = %q, want it to cite the iam_check step", flow.BlockedReason)
	}
}

// TestComputeJourneyFlow_NoIAMCheck_Unaffected proves a journey declaring no
// IAMCheck at all is evaluated exactly as PC-125 left it — this ticket's own
// additive-only guarantee.
func TestComputeJourneyFlow_NoIAMCheck_Unaffected(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	if !flow.Flows {
		t.Fatalf("got Flows=false, want true: %+v", flow)
	}
}
