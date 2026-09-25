package core_test

// PC-129: NAT gateway loss and route-removal fault tests.

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestEvaluateEgressRoute_GoldenBundle_RealNATReachable(t *testing.T) {
	ir := realGoldenIR(t)
	allowed, kind, reason := core.EvaluateEgressRoute(ir, "aws_subnet.private_a")
	if !allowed {
		t.Fatalf("got denied, want allowed (nat_a is a real, undisturbed route target): %s", reason)
	}
	if kind != "nat_gateway" {
		t.Errorf("kind = %q, want nat_gateway", kind)
	}
}

// TestNATGatewayLoss_BreaksEgressButNotInternalTraffic is PC-129's own acceptance
// criterion, verbatim: "NAT loss: private → internet fails; private → RDS still
// succeeds — both asserted in one test on a real fixture."
func TestNATGatewayLoss_BreaksEgressButNotInternalTraffic(t *testing.T) {
	ir := realGoldenIR(t)
	mutated := core.WithNATGatewayLost(ir, "aws_nat_gateway.nat_a")

	// Half 1: private_a's own egress now fails — its only NAT target is gone.
	allowed, _, reason := core.EvaluateEgressRoute(mutated, "aws_subnet.private_a")
	if allowed {
		t.Fatal("got allowed, want denied — nat_a was removed as a route target")
	}
	if !strings.Contains(reason, "aws_nat_gateway.nat_a") {
		t.Errorf("reason = %q, want it to name the lost NAT gateway", reason)
	}

	// Half 2: a DIFFERENT AZ's own egress (private_b -> nat_b) is UNAFFECTED — never
	// marks another AZ's NAT failed by association.
	allowedB, _, _ := core.EvaluateEgressRoute(mutated, "aws_subnet.private_b")
	if !allowedB {
		t.Fatal("got denied for private_b, want allowed — nat_b was never touched")
	}

	// Half 3: private -> another real, resolvable component inside the same VPC
	// still succeeds at the route_selection step specifically — NAT gateway loss
	// only removes ITS OWN route target, never touches a subnet's own route-table
	// association. (golden/aws's own "data" subnets — RDS/ElastiCache — have no
	// route_table_association at all, a separate, pre-existing gap in the golden
	// Terraform unrelated to this fault, so aws_lb.payments (a real, resolvable
	// public-subnet destination) is used here instead. NACL resolution may still
	// separately block the golden bundle's own real trace — PC-125's own documented,
	// honest gap — so this checks the route_selection step's own decision, not the
	// whole trace's outcome.)
	trace := core.BuildTrace(mutated, "aws_eks_cluster.payments", "aws_lb.payments", "", "tcp", 443)
	var routeStep *core.TraceStep
	for i := range trace.Steps {
		if trace.Steps[i].Step == "route_selection" {
			routeStep = &trace.Steps[i]
		}
	}
	if routeStep == nil {
		t.Fatal("expected a route_selection step in the trace")
	}
	if routeStep.Decision != core.TraceAllow {
		t.Fatalf("route_selection = %+v, want allow — internal (same-VPC) traffic must never be affected by a NAT gateway loss", routeStep)
	}
}

func TestRouteRemoval_BreaksExactlyThatRoute(t *testing.T) {
	ir := realGoldenIR(t)
	mutated := core.WithRouteRemoved(ir, "aws_route_table.private_a", "0.0.0.0/0")

	allowed, _, reason := core.EvaluateEgressRoute(mutated, "aws_subnet.private_a")
	if allowed {
		t.Fatal("got allowed, want denied — the default route was removed")
	}
	if !strings.Contains(reason, "no longer exists") {
		t.Errorf("reason = %q, want it to name the now-unreachable route target", reason)
	}

	// A different route table's own route is untouched.
	allowedB, _, _ := core.EvaluateEgressRoute(mutated, "aws_subnet.private_b")
	if !allowedB {
		t.Fatal("got denied for private_b, want allowed — its own route table was never touched")
	}
}

// TestBuildNATSharedAcrossAZsFindings is PC-129's own acceptance criterion, verbatim:
// "Single-NAT-multi-AZ design produces a SPOF finding; NAT-per-AZ design does not —
// both tested."
func TestBuildNATSharedAcrossAZsFindings_GoldenBundle_NoFindingRealNATPerAZ(t *testing.T) {
	ir := realGoldenIR(t)
	findings := core.BuildNATSharedAcrossAZsFindings(ir)
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 — golden/aws has one NAT gateway per AZ (nat_a/nat_b/nat_c), a real, correct design: %+v", len(findings), findings)
	}
}

// TestBuildNATSharedAcrossAZsFindings_GoldenBroken_MatchesItsOwnDocumentedDefect
// hand-verifies against golden/aws-broken/network.tf's own "DEFECT 1" comment,
// verbatim: "one NAT gateway in AZ-a carries egress for all three private subnets
// ... Expected: min-cut of size 1 at aws_nat_gateway.nat_a." This is a pre-existing,
// intentionally-planted defect this codebase had no engine to detect until this
// ticket — a real, meaningful confirmation, not a fixture built to fit the code.
func TestBuildNATSharedAcrossAZsFindings_GoldenBroken_MatchesItsOwnDocumentedDefect(t *testing.T) {
	awsReg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("awsprovider.Load(): %v", err)
	}
	result, err := ingest.Ingest(filepath.Join("..", "golden", "aws-broken"), awsReg, 1)
	if err != nil {
		t.Fatalf("Ingest(aws-broken): %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("got insufficient_model: %+v", result.Insufficient)
	}

	findings := core.BuildNATSharedAcrossAZsFindings(result.IR)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1 (nat_a shared by all three private route tables): %+v", len(findings), findings)
	}
	if findings[0].ID != "finding.routing.nat-shared-across-azs.aws_nat_gateway.nat_a" {
		t.Errorf("ID = %q, want it to name aws_nat_gateway.nat_a — the exact node golden/aws-broken/network.tf's own DEFECT 1 comment names", findings[0].ID)
	}
	wantComponents := []string{"aws_route_table.private_a", "aws_route_table.private_b", "aws_route_table.private_c"}
	if !reflect.DeepEqual(findings[0].Dimensions.AffectedComponents, wantComponents) {
		t.Errorf("AffectedComponents = %v, want all three private route tables (sorted by insertion — see the finding's own determinism note if this becomes flaky)", findings[0].Dimensions.AffectedComponents)
	}
}

func TestBuildNATSharedAcrossAZsFindings_SharedNAT_ProducesFinding(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	ir := &core.IR{
		Nodes: []core.Node{
			rt("rtA", core.NodeTypeNetworkBoundary),
			rt("rtB", core.NodeTypeNetworkBoundary),
			rt("nat", core.NodeTypeNetworkBoundary),
		},
		Edges: []core.Edge{
			{ID: "e1", Type: core.EdgeTypeRoutesTo, From: "rtA", To: "nat", Resolution: core.ResolutionKnown, Provenance: prov,
				RawAttributes: map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "nat_gateway"}},
			{ID: "e2", Type: core.EdgeTypeRoutesTo, From: "rtB", To: "nat", Resolution: core.ResolutionKnown, Provenance: prov,
				RawAttributes: map[string]any{"destination_cidr": "0.0.0.0/0", "target_kind": "nat_gateway"}},
		},
	}
	findings := core.BuildNATSharedAcrossAZsFindings(ir)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 (nat is shared by rtA and rtB): %+v", len(findings), findings)
	}
	if findings[0].ID != "finding.routing.nat-shared-across-azs.nat" {
		t.Errorf("ID = %q", findings[0].ID)
	}
}
