package analyse

import "testing"

// TestDetectSPOFs_SingleNATGatewayShape hand-verifies DetectSPOFs against a graph
// shaped exactly like golden/aws-broken's defect 1 (one NAT gateway serving three
// private subnets): entry points are the three subnets, the critical node is the
// internet gateway/external target. Hand-computed: N is a SPOF for all three subnets
// (CutSize 1 each).
func TestDetectSPOFs_SingleNATGatewayShape(t *testing.T) {
	edges := []DirectedEdge{
		{"subnet_a", "N"}, {"subnet_b", "N"}, {"subnet_c", "N"},
		{"N", "internet"},
	}
	candidates := DetectSPOFs(edges, []string{"subnet_a", "subnet_b", "subnet_c"}, []string{"internet"})

	if len(candidates) != 3 {
		t.Fatalf("got %d candidates, want 3 (one per entry point)", len(candidates))
	}
	for _, c := range candidates {
		if !c.IsSPOF() {
			t.Errorf("%s -> %s: got %+v, want IsSPOF()==true", c.EntryPoint, c.CriticalNode, c)
		}
		if len(c.CutVertices) != 1 || c.CutVertices[0] != "N" {
			t.Errorf("%s -> %s: CutVertices = %v, want [N]", c.EntryPoint, c.CriticalNode, c.CutVertices)
		}
	}
}

// TestDetectSPOFs_ThreeIndependentNATGateways_NotFlaggedAsSPOFs is the resilient
// counterpart to golden/aws (the CLEAN bundle): one NAT gateway per AZ. Hand-computed:
// each subnet's OWN path still shows CutSize 1 (its own gateway) — this is expected
// and correct (see mincut_test.go's own note on this), and is why SPOF detection must
// be paired with the redundancy/AZ-diversity question PC-14's Card separately names,
// not read as "any CutSize-1 result is automatically alarming" on its own.
func TestDetectSPOFs_ThreeIndependentNATGateways_EachStillShowsOwnGateway(t *testing.T) {
	edges := []DirectedEdge{
		{"subnet_a", "N1"}, {"N1", "internet"},
		{"subnet_b", "N2"}, {"N2", "internet"},
		{"subnet_c", "N3"}, {"N3", "internet"},
	}
	candidates := DetectSPOFs(edges, []string{"subnet_a"}, []string{"internet"})
	if len(candidates) != 1 || !candidates[0].IsSPOF() || candidates[0].CutVertices[0] != "N1" {
		t.Fatalf("got %+v, want a single-vertex cut at N1 (subnet_a's own, and only, gateway)", candidates)
	}
}

// TestDetectSPOFs_RedundantGateways_NotAPOF confirms a genuinely resilient design
// (two independent gateways serving the SAME subnet) is correctly NOT flagged.
func TestDetectSPOFs_RedundantGateways_NotASPOF(t *testing.T) {
	edges := []DirectedEdge{
		{"subnet_a", "N1"}, {"N1", "internet"},
		{"subnet_a", "N2"}, {"N2", "internet"},
	}
	candidates := DetectSPOFs(edges, []string{"subnet_a"}, []string{"internet"})
	if len(candidates) != 1 || candidates[0].IsSPOF() {
		t.Fatalf("got %+v, want IsSPOF()==false (two independent gateways)", candidates)
	}
	if candidates[0].CutSize != 2 {
		t.Errorf("CutSize = %d, want 2", candidates[0].CutSize)
	}
}

// TestDetectSPOFs_NoPath_NotASPOF is the honesty guard IsSPOF's own doc comment
// promises: an absent dependency (no edge at all between entry and critical node) must
// never be reported as a SPOF, and must be distinguishable from "genuinely resilient."
func TestDetectSPOFs_NoPath_NotASPOF(t *testing.T) {
	edges := []DirectedEdge{{"A", "B"}} // no connection to "critical" at all
	candidates := DetectSPOFs(edges, []string{"A"}, []string{"critical"})
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	if candidates[0].IsSPOF() {
		t.Fatal("no-path candidate must not be reported as a SPOF")
	}
	if candidates[0].CutSize != 0 || candidates[0].Uncuttable {
		t.Errorf("got %+v, want CutSize=0, Uncuttable=false (vacuous, not resilient, not a SPOF)", candidates[0])
	}
}

func TestDetectSPOFs_EntryEqualsCritical_Skipped(t *testing.T) {
	candidates := DetectSPOFs([]DirectedEdge{{"A", "B"}}, []string{"A"}, []string{"A"})
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 — a node is never a SPOF for itself", len(candidates))
	}
}
