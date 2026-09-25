package core_test

// PC-135's second acceptance criterion: "iam_policy_change fault breaks the expected
// journeys and nothing else — hand-verified." First proves the pure IR mutation
// (WithIAMStatementRemoved/WithIAMDenyStatementAdded) in isolation, then proves the
// full Simulate() integration: a journey depending on the changed policy breaks,
// while an unrelated journey (a different principal, same network path) is
// unaffected — hand-verified against a synthetic fixture, same discipline as PC-129's
// own NAT-loss test.

import (
	"testing"

	"preflight/core"
)

func TestWithIAMStatementRemoved(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	mutated, ok := core.WithIAMStatementRemoved(ir, "policy", "AllowGet")
	if !ok {
		t.Fatal("got ok=false, want true")
	}
	if len(mutated.Nodes[0].IAMIdentityPolicies[0].Statements) != 0 {
		t.Fatalf("mutated statements = %+v, want empty", mutated.Nodes[0].IAMIdentityPolicies[0].Statements)
	}
	// The original ir must be untouched — copy-on-write, never mutate in place.
	if len(ir.Nodes[0].IAMIdentityPolicies[0].Statements) != 1 {
		t.Fatal("original ir was mutated in place — WithIAMStatementRemoved must return a copy")
	}
}

func TestWithIAMStatementRemoved_UnknownSid_NotOK(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"s3:GetObject"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}
	_, ok := core.WithIAMStatementRemoved(ir, "policy", "NoSuchSid")
	if ok {
		t.Fatal("got ok=true, want false — refusing to guess which statement was meant")
	}
}

func TestWithIAMDenyStatementAdded_EffectAlwaysForcedToDeny(t *testing.T) {
	role := identityNode("role", identityPolicy("policy",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"*"}},
	))
	ir := &core.IR{Nodes: []core.Node{role}}

	mutated, ok := core.WithIAMDenyStatementAdded(ir, "policy", core.PolicyStatement{
		Sid: "InjectedDeny", Effect: "Allow", // deliberately wrong — must be forced to Deny
		Action: []string{"s3:GetObject"}, Resource: []string{"*"},
	})
	if !ok {
		t.Fatal("got ok=false, want true")
	}
	stmts := mutated.Nodes[0].IAMIdentityPolicies[0].Statements
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2 (original + injected)", len(stmts))
	}
	if stmts[1].Effect != "Deny" {
		t.Fatalf("injected statement Effect = %q, want Deny (forced, regardless of what was supplied)", stmts[1].Effect)
	}
}

// TestSimulate_IAMPolicyChangeFault_BreaksOnlyDependentJourney is the Card's own
// acceptance criterion, verbatim: "iam_policy_change fault breaks the expected
// journeys and nothing else." journeyA depends on "policy"'s AllowGet statement;
// journeyB's principal has a SEPARATE, untouched policy — removing journeyA's own
// statement must break journeyA's flow and leave journeyB's flow exactly as it was.
func TestSimulate_IAMPolicyChangeFault_BreaksOnlyDependentJourney(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	roleA := identityNode("roleA", identityPolicy("policyA",
		core.PolicyStatement{Sid: "AllowGet", Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	roleB := identityNode("roleB", identityPolicy("policyB",
		core.PolicyStatement{Sid: "AllowGetB", Effect: "Allow", Action: []string{"dynamodb:GetItem"}, Resource: []string{"*"}},
	))
	ir.Nodes = append(ir.Nodes, roleA, roleB)

	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "journeyA", Name: "journeyA", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
				IAMCheck: &core.JourneyIAMCheck{PrincipalID: "roleA", Action: "dynamodb:GetItem", ResourceARN: "*"}},
			{ID: "journeyB", Name: "journeyB", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1",
				IAMCheck: &core.JourneyIAMCheck{PrincipalID: "roleB", Action: "dynamodb:GetItem", ResourceARN: "*"}},
		},
	}

	before := core.Simulate(ir, workload, nil, syntheticProv())
	for _, f := range before.FlowDetail {
		if !f.Flows {
			t.Fatalf("before the fault, journey %q unexpectedly does not flow: %+v", f.JourneyID, f)
		}
	}

	after := core.Simulate(ir, workload, []core.Fault{
		{Type: "iam_policy_change", Target: "policyA", IAMRemoveStatementSid: "AllowGet"},
	}, syntheticProv())

	var flowA, flowB *core.JourneyFlowResult
	for i := range after.FlowDetail {
		switch after.FlowDetail[i].JourneyID {
		case "journeyA":
			flowA = &after.FlowDetail[i]
		case "journeyB":
			flowB = &after.FlowDetail[i]
		}
	}
	if flowA == nil || flowB == nil {
		t.Fatalf("expected both journeys in FlowDetail, got %+v", after.FlowDetail)
	}
	if flowA.Flows {
		t.Error("journeyA.Flows = true, want false — its own policy's only Allow statement was removed")
	}
	if !flowB.Flows {
		t.Errorf("journeyB.Flows = false, want true — its policy was never touched: %+v", flowB)
	}
}

func TestResolveFaults_IAMPolicyChange_UnknownTarget_ReturnsError(t *testing.T) {
	ir := &core.IR{}
	resp := core.Simulate(ir, core.Workload{}, []core.Fault{
		{Type: "iam_policy_change", Target: "no-such-policy", IAMRemoveStatementSid: "x"},
	}, syntheticProv())
	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Verdict.State = %q, want not_assessable — refusing to guess which policy was meant", resp.Verdict.State)
	}
}

func TestResolveFaults_IAMPolicyChange_RequiresExactlyOneMutation(t *testing.T) {
	ir := &core.IR{}
	resp := core.Simulate(ir, core.Workload{}, []core.Fault{
		{Type: "iam_policy_change", Target: "policy"}, // neither remove nor add specified
	}, syntheticProv())
	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Verdict.State = %q, want not_assessable", resp.Verdict.State)
	}
}
