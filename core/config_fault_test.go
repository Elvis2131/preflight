package core_test

// PC-130's own acceptance criteria: "Each new fault type mutates the model and
// re-runs the request engine; trace shows the changed rule as the deciding step" and
// "Removing the one SG rule that allows ECS -> RDS breaks the checkout journey, and
// the trace names that rule — tested on a golden fixture."
//
// GOLDEN-FIXTURE NOTE (revised under PC-149): golden/aws declares no NACLs, so each
// subnet is on its VPC's default NACL — AWS's documented allow-all, assumed unmodified
// — and full BuildTrace calls into golden/aws now reach their SG decisions. The two real
// golden hops that can be traced end to end are internet -> ALB (tcp/443) and
// ALB -> workload (tcp/8080); the golden checkout JOURNEY still cannot flow as a whole
// (it declares one port, tcp/443, for every hop) and EKS -> RDS stops earlier at route
// selection (golden's data subnets have no explicit route-table association, and the
// implicit main route table is not modelled — a separate gap). The older tests below
// that exercise SecurityGroupProfile/EvaluateConnection directly stay valid: that is
// the exact engine BuildTrace's sg_dest_ingress step calls.

import (
	"testing"

	"preflight/core"
)

func TestWithSGRuleRemoved_GoldenBundle_RealWorkloadToDatabaseRule(t *testing.T) {
	ir := realGoldenIR(t)

	// The exact real rule golden/aws/security.tf declares (aws_security_group_rule.
	// workload_to_database's own ingress mirror on aws_security_group.database).
	rule := core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, SourceSG: "aws_security_group.workload"}

	// SecurityGroupProfile takes a RESOURCE's own ID (it walks that resource's own
	// depends_on edges to find its attached SGs) — aws_db_instance.payments and
	// aws_eks_cluster.payments are the real golden resources depends_on-attached to
	// the database/workload SGs respectively (golden/aws/rds.tf, eks.tf), not the SG
	// node IDs themselves.
	before := core.SecurityGroupProfile(ir.Nodes, ir.Edges, "aws_db_instance.payments")
	beforeAllowed, _, _ := core.EvaluateConnection(
		core.SecurityGroupProfile(ir.Nodes, ir.Edges, "aws_eks_cluster.payments"), before, "", "", "tcp", 5432)
	if !beforeAllowed {
		t.Fatal("before removal: got denied, want allowed — this is golden/aws's own real, declared rule")
	}

	mutated, ok := core.WithSGRuleRemoved(ir, "aws_security_group.database", rule)
	if !ok {
		t.Fatal("got ok=false — the exact real golden rule was not found; check it against security.tf")
	}

	after := core.SecurityGroupProfile(mutated.Nodes, mutated.Edges, "aws_db_instance.payments")
	afterAllowed, _, _ := core.EvaluateConnection(
		core.SecurityGroupProfile(mutated.Nodes, mutated.Edges, "aws_eks_cluster.payments"), after, "", "", "tcp", 5432)
	if afterAllowed {
		t.Fatal("after removal: got allowed, want denied — the only rule permitting this connection was removed")
	}

	// The original ir must be untouched.
	stillAllowed, _, _ := core.EvaluateConnection(
		core.SecurityGroupProfile(ir.Nodes, ir.Edges, "aws_eks_cluster.payments"),
		core.SecurityGroupProfile(ir.Nodes, ir.Edges, "aws_db_instance.payments"), "", "", "tcp", 5432)
	if !stillAllowed {
		t.Fatal("original ir was mutated in place — WithSGRuleRemoved must return a copy")
	}
}

// TestSGRuleRemoval_GoldenBundle_BreaksALBIngress_SGStepNamesTheFailure hand-verifies
// the same real golden ALB ingress rule (security.tf) at the SG-evaluation layer
// directly, rather than via a full BuildTrace call — see this file's own honesty note
// above for why: golden/aws has zero NACL resources anywhere, so EVERY full
// BuildTrace call into golden/aws is blocked at nacl_check before it can ever reach
// an sg_dest_ingress decision, regardless of any SG fault. EvaluateConnection is the
// exact function BuildTrace's own sg_dest_ingress step calls (PC-112) — this proves
// the real rule change propagates correctly through the real engine.
func TestSGRuleRemoval_GoldenBundle_BreaksALBIngress_SGStepNamesTheFailure(t *testing.T) {
	ir := realGoldenIR(t)

	rule := core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}}
	mutated, ok := core.WithSGRuleRemoved(ir, "aws_security_group.alb", rule)
	if !ok {
		t.Fatal("got ok=false — the real golden ALB ingress rule was not found; check it against security.tf")
	}

	albProfile := core.SecurityGroupProfile(ir.Nodes, ir.Edges, "aws_lb.payments")
	internetProfile := core.SGProfile{Rules: []core.SGRule{{Direction: "egress", Protocol: "-1", CIDRs: []string{"0.0.0.0/0"}}}}
	beforeAllowed, _, beforeResponder := core.EvaluateConnection(internetProfile, albProfile, "0.0.0.0/0", "0.0.0.0/0", "tcp", 443)
	if !beforeAllowed {
		t.Fatal("before removal: got denied, want allowed — this is golden/aws's own real, declared rule")
	}

	mutatedALBProfile := core.SecurityGroupProfile(mutated.Nodes, mutated.Edges, "aws_lb.payments")
	afterAllowed, _, afterResponder := core.EvaluateConnection(internetProfile, mutatedALBProfile, "0.0.0.0/0", "0.0.0.0/0", "tcp", 443)
	if afterAllowed {
		t.Fatalf("after removal: got allowed, want denied — the only rule permitting HTTPS ingress was removed (before responder decision: %+v, after: %+v)", beforeResponder, afterResponder)
	}
}

func TestWithSGRuleAdded_DenyBySGProfileAbsence(t *testing.T) {
	// SGRule has no "Deny" concept of its own (AWS SGs are allow-only, PC-112's own
	// documented model) — adding a rule can only ever WIDEN what is allowed, never
	// narrow it. This proves sg_rule_add does exactly that: a connection that was
	// denied before becomes allowed after.
	ir := buildFlowTestIR(false, true) // SG denies port 5432
	newRule := core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"10.0.1.0/24"}}

	before := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if before.Allowed {
		t.Fatal("before add: got Allowed=true, want false")
	}

	mutated, ok := core.WithSGRuleAdded(ir, "sgDb", newRule)
	if !ok {
		t.Fatal("got ok=false, want true — sgDb is a real node in this fixture")
	}
	after := core.BuildTrace(mutated, "app", "db", "", "tcp", 5432)
	if !after.Allowed {
		t.Fatalf("after add: got Allowed=false, want true: %+v", after.Steps)
	}
}

func TestWithNACLRuleRemoved_BreaksJourney_Synthetic(t *testing.T) {
	ir := buildFlowTestIR(true, true) // fully flows at baseline
	// naclA's own allow-all rules (see buildFlowTestIR's naclRules helper) — remove
	// the egress rule, which must break subnetA -> subnetB traffic.
	rule := core.NACLRule{Number: 100, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true}

	before := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if !before.Allowed {
		t.Fatalf("before removal: got Allowed=false, want true: %+v", before.Steps)
	}

	mutated, ok := core.WithNACLRuleRemoved(ir, "naclA", rule)
	if !ok {
		t.Fatal("got ok=false — the fixture's own naclA egress rule was not found")
	}
	after := core.BuildTrace(mutated, "app", "db", "", "tcp", 5432)
	if after.Allowed {
		t.Fatal("after removal: got Allowed=true, want false — naclA no longer permits any egress")
	}
}

func TestWithNACLRuleRemoved_UnknownRule_NotOK(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	_, ok := core.WithNACLRuleRemoved(ir, "naclA", core.NACLRule{Number: 999, Direction: "ingress", Protocol: "tcp"})
	if ok {
		t.Fatal("got ok=true, want false — refusing to guess which rule was meant")
	}
}

func TestResolveFaults_SGRuleChange_RequiresExactlyOneMutation(t *testing.T) {
	ir := &core.IR{}
	resp := core.Simulate(ir, core.Workload{}, []core.Fault{
		{Type: "sg_rule_change", Target: "sg"},
	}, syntheticProv())
	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Fatalf("Verdict.State = %q, want not_assessable", resp.Verdict.State)
	}
}

// TestSimulate_SGRuleChangeFault_BreaksOnlyDependentJourney is PC-130's own
// end-to-end proof — the exact semantic the Card's named scenario describes (a rule
// removal breaks the journey depending on it), on a synthetic fixture free of
// golden's own unrelated route/NACL gaps documented above.
func TestSimulate_SGRuleChangeFault_BreaksOnlyDependentJourney(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{
		Journeys: []core.DeclaredJourney{
			{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"},
		},
	}

	before := core.Simulate(ir, workload, nil, syntheticProv())
	if len(before.FlowDetail) != 1 || !before.FlowDetail[0].Flows {
		t.Fatalf("before the fault, journey unexpectedly does not flow: %+v", before.FlowDetail)
	}

	after := core.Simulate(ir, workload, []core.Fault{
		{Type: "sg_rule_change", Target: "sgDb", SGRuleRemove: &core.SGRule{
			Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"10.0.1.0/24"},
		}},
	}, syntheticProv())
	if len(after.FlowDetail) != 1 || after.FlowDetail[0].Flows {
		t.Fatalf("after the fault, journey should no longer flow: %+v", after.FlowDetail)
	}
}

// lbFlowIR is buildFlowTestIR with "app" turned into a load balancer that explicitly
// serves traffic to "db" (the canvas's own LB routes_to target fact). Everything else
// — SGs, NACLs, subnets — is the fully-flowing synthetic baseline.
func lbFlowIR(withTargetEdge bool) *core.IR {
	ir := buildFlowTestIR(true, true)
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == "app" {
			ir.Nodes[i].Type = core.NodeTypeLoadBalancer
		}
	}
	if withTargetEdge {
		ir.Edges = append(ir.Edges, core.Edge{ID: "lb-target", Type: core.EdgeTypeRoutesTo, From: "app", To: "db", Resolution: core.ResolutionKnown, Provenance: syntheticProv()})
	}
	return ir
}

func TestTargetDeregistration_BreaksJourney_TraceNamesRegistrationStep(t *testing.T) {
	ir := lbFlowIR(true)
	before := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if !before.Allowed {
		t.Fatalf("baseline must flow with db registered: %+v", before.Steps)
	}
	sawRegistered := false
	for _, s := range before.Steps {
		if s.Step == "target_registration" && s.Decision == core.TraceAllow {
			sawRegistered = true
		}
	}
	if !sawRegistered {
		t.Error("baseline trace should show an allowing target_registration step")
	}

	workload := core.Workload{Journeys: []core.DeclaredJourney{{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}}}
	after := core.Simulate(ir, workload, []core.Fault{{Type: "target_deregistration", Target: "app", DeregisterTarget: "db"}}, syntheticProv())
	if len(after.FlowDetail) != 1 || after.FlowDetail[0].Flows {
		t.Fatalf("after deregistration the journey must not flow: %+v", after.FlowDetail)
	}

	mutated, ok := core.WithTargetDeregistered(ir, "app", "db")
	if !ok {
		t.Fatal("WithTargetDeregistered returned ok=false for a real registration")
	}
	tr := core.BuildTrace(mutated, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatal("trace must deny a deregistered target")
	}
	var deciding string
	for _, s := range tr.Steps {
		if s.Decision == core.TraceDeny {
			deciding = s.Step
		}
	}
	if deciding != "target_registration" {
		t.Errorf("deciding step = %q, want target_registration", deciding)
	}
	// Copy-on-write: the original IR still flows.
	if !core.BuildTrace(ir, "app", "db", "", "tcp", 5432).Allowed {
		t.Error("original IR was mutated")
	}
}

// With no modelled registration (the HCL case) the fault is refused, and a trace
// makes no registration claim — never a guessed pass or fail (I4).
func TestTargetDeregistration_NoModelledRegistration_RefusedAndTraceUnchanged(t *testing.T) {
	ir := lbFlowIR(false)
	if _, ok := core.WithTargetDeregistered(ir, "app", "db"); ok {
		t.Error("deregistering an unmodelled registration must be refused")
	}
	for _, s := range core.BuildTrace(ir, "app", "db", "", "tcp", 5432).Steps {
		if s.Step == "target_registration" {
			t.Errorf("no registration is modelled; trace must not claim one: %+v", s)
		}
	}
	resp := core.Simulate(ir, core.Workload{}, []core.Fault{{Type: "target_deregistration", Target: "app", DeregisterTarget: "db"}}, syntheticProv())
	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable", resp.Verdict.State)
	}
	// A non-load-balancer target is refused too.
	if _, ok := core.WithTargetDeregistered(buildFlowTestIR(true, true), "app", "db"); ok {
		t.Error("a compute node is not a load balancer")
	}
}

// PC-130's golden-fixture criterion, re-run under PC-149: removing the ONE real SG rule
// that admits a hop breaks that hop through the full pipeline, and the trace names the
// security group step and the group that decided.
func goldenSGDecision(t *testing.T, ir *core.IR, src, dest string, port int) (core.Trace, core.TraceStep) {
	t.Helper()
	cidr := ""
	if src == "" {
		cidr = "0.0.0.0/0"
	}
	tr := core.BuildTrace(ir, src, dest, cidr, "tcp", port)
	for _, s := range tr.Steps {
		if s.Step == "sg_dest_ingress" {
			return tr, s
		}
	}
	t.Fatalf("no sg_dest_ingress step: %+v", tr.Steps)
	return tr, core.TraceStep{}
}

func TestGolden_FullTrace_RemovingTheRealSGRuleBreaksTheHop(t *testing.T) {
	ir := realGoldenIR(t)
	cases := []struct {
		name, src, dest, sg string
		port                int
		rule                core.SGRule
	}{
		{"internet -> ALB", "", "aws_lb.payments", "aws_security_group.alb", 443,
			core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"}}},
		{"ALB -> workload", "aws_lb.payments", "aws_eks_cluster.payments", "aws_security_group.workload", 8080,
			core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 8080, ToPort: 8080, SourceSG: "aws_security_group.alb"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before, _ := goldenSGDecision(t, ir, c.src, c.dest, c.port)
			if !before.Allowed {
				t.Fatalf("baseline hop must reach Allowed on golden/aws: %+v", before.Steps)
			}
			mutated, ok := core.WithSGRuleRemoved(ir, c.sg, c.rule)
			if !ok {
				t.Fatalf("the real golden rule on %s was not found; check it against security.tf", c.sg)
			}
			after, step := goldenSGDecision(t, mutated, c.src, c.dest, c.port)
			if after.Allowed || step.Decision != core.TraceDeny {
				t.Fatalf("after removing the only admitting rule the hop must be denied at sg_dest_ingress: %+v", after.Steps)
			}
			if !core.BuildTrace(ir, c.src, c.dest, map[bool]string{true: "0.0.0.0/0"}[c.src == ""], "tcp", c.port).Allowed {
				t.Error("original IR was mutated")
			}
		})
	}
}

// The same break observed as a journey through Simulate: a golden journey that CAN
// flow (internet -> ALB on its declared tcp/443) stops flowing under sg_rule_change.
func TestGolden_Simulate_SGRuleChange_BreaksFlowingJourney(t *testing.T) {
	ir := realGoldenIR(t)
	workload := core.Workload{Journeys: []core.DeclaredJourney{
		{ID: "edge", Name: "edge", Path: []string{core.JourneyInternetSentinel, "aws_lb.payments"}, Protocol: "tcp", Port: 443, Criticality: "tier1"},
	}}
	before := core.Simulate(ir, workload, nil, syntheticProv())
	if len(before.FlowDetail) != 1 || !before.FlowDetail[0].Flows {
		t.Fatalf("golden internet -> ALB journey must flow at baseline: %+v", before.FlowDetail)
	}
	after := core.Simulate(ir, workload, []core.Fault{{Type: "sg_rule_change", Target: "aws_security_group.alb", SGRuleRemove: &core.SGRule{
		Direction: "ingress", Protocol: "tcp", FromPort: 443, ToPort: 443, CIDRs: []string{"0.0.0.0/0"},
	}}}, syntheticProv())
	if len(after.FlowDetail) != 1 || after.FlowDetail[0].Flows {
		t.Fatalf("after removing the ALB's 443 rule the journey must stop flowing: %+v", after.FlowDetail)
	}
}
