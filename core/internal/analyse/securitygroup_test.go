package analyse

import "testing"

// TestFilterEdgesBySGRules_RevealsSPOFPureTopologyMisses is PC-79's second acceptance
// criterion, hand-verified on exactly the synthetic shape it names: "a SPOF caused
// purely by an SG rule gap (topology otherwise fully connected) is detected... before
// touching the golden architecture."
//
// Shape: entry has TWO topologically independent routes to data (via compute1 and
// compute2) — pure topology analysis (DetectSPOFs on the unfiltered edges) correctly
// reports this as resilient, CutSize 2, not a SPOF. But compute2's security group is
// missing the rule data's SG would need to permit it in (a real SG misconfiguration,
// not a missing edge) — data only accepts inbound from compute1's SG. Once
// FilterEdgesBySGRules drops the compute2->data edge that rule gap actually blocks, the
// SAME topology collapses to a single real path, and DetectSPOFs — called completely
// unchanged, PC-14's own function — now correctly reports compute1 as a real SPOF that
// pure topology analysis alone would have missed entirely.
func TestFilterEdgesBySGRules_RevealsSPOFPureTopologyMisses(t *testing.T) {
	edges := []DirectedEdge{
		{"entry", "compute1"}, {"compute1", "data"},
		{"entry", "compute2"}, {"compute2", "data"},
	}
	// Mirrors golden/aws/security.tf's real alb -> workload -> database chain shape:
	// every hop is gated by an explicit SG-to-SG allow rule, entry included, so the
	// only gap under test is the one deliberately left out below.
	sgOf := map[string][]string{
		"entry":    {"sg-entry"},
		"compute1": {"sg-compute1"},
		"compute2": {"sg-compute2"},
		"data":     {"sg-data"},
	}
	sgRules := map[string][]SGRule{
		"sg-compute1": {{SourceSG: "sg-entry"}},
		"sg-compute2": {{SourceSG: "sg-entry"}},
		// data's SG only permits inbound from compute1's SG — compute2's SG was never
		// added to this rule, the real-world misconfiguration this ticket is about.
		"sg-data": {{SourceSG: "sg-compute1"}},
	}

	unfiltered := DetectSPOFs(edges, []string{"entry"}, []string{"data"})
	if len(unfiltered) != 1 || unfiltered[0].IsSPOF() || unfiltered[0].CutSize != 2 {
		t.Fatalf("pure topology: got %+v, want CutSize=2, IsSPOF()=false (two independent routes)", unfiltered)
	}

	filtered := FilterEdgesBySGRules(edges, sgOf, sgRules)
	afterSG := DetectSPOFs(filtered, []string{"entry"}, []string{"data"})
	if len(afterSG) != 1 {
		t.Fatalf("got %d candidates, want 1", len(afterSG))
	}
	if !afterSG[0].IsSPOF() {
		t.Fatalf("after SG filtering: got %+v, want IsSPOF()==true — compute2's route is blocked by the SG rule gap, leaving only compute1", afterSG[0])
	}
	if len(afterSG[0].CutVertices) != 1 || afterSG[0].CutVertices[0] != "compute1" {
		t.Errorf("CutVertices = %v, want [compute1]", afterSG[0].CutVertices)
	}
}

// TestFilterEdgesBySGRules_NoSGDataForDestination_PassesThroughUnchanged is the I4-
// style honesty guard this file's own doc comment promises: an edge whose destination
// has no SG data at all must never be silently treated as blocked.
func TestFilterEdgesBySGRules_NoSGDataForDestination_PassesThroughUnchanged(t *testing.T) {
	edges := []DirectedEdge{{"a", "b"}}
	got := FilterEdgesBySGRules(edges, map[string][]string{}, map[string][]SGRule{})
	if len(got) != 1 || got[0] != edges[0] {
		t.Fatalf("got %v, want the edge passed through unchanged (no SG data recorded for \"b\")", got)
	}
}

// TestFilterEdgesBySGRules_SourceSGMatch_Permitted confirms the positive case
// independently of the SPOF scenario above: a correctly configured rule permits the
// edge to pass through.
func TestFilterEdgesBySGRules_SourceSGMatch_Permitted(t *testing.T) {
	edges := []DirectedEdge{{"compute", "data"}}
	sgOf := map[string][]string{"compute": {"sg-compute"}, "data": {"sg-data"}}
	sgRules := map[string][]SGRule{"sg-data": {{SourceSG: "sg-compute"}}}

	got := FilterEdgesBySGRules(edges, sgOf, sgRules)
	if len(got) != 1 {
		t.Fatalf("got %v, want the edge to pass through — sg-data's rule permits sg-compute", got)
	}
}

// TestFilterEdgesBySGRules_NoMatchingRule_Blocked is the direct negative case: the
// destination DOES have SG data, but none of its rules name the source's SG.
func TestFilterEdgesBySGRules_NoMatchingRule_Blocked(t *testing.T) {
	edges := []DirectedEdge{{"compute", "data"}}
	sgOf := map[string][]string{"compute": {"sg-compute"}, "data": {"sg-data"}}
	sgRules := map[string][]SGRule{"sg-data": {{SourceSG: "sg-someone-else"}}}

	got := FilterEdgesBySGRules(edges, sgOf, sgRules)
	if len(got) != 0 {
		t.Fatalf("got %v, want the edge blocked — sg-data has rules, but none name sg-compute", got)
	}
}
