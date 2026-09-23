package analyse

import "reflect"

import "testing"

// A -> N -> D (single chokepoint). Killing N loses D (D is only reachable through N).
// Hand-computed: lost = [D].
func TestSimulateLoss_SingleChokepoint_LosesDownstream(t *testing.T) {
	edges := []DirectedEdge{{"A", "N"}, {"N", "D"}}
	lost := SimulateLoss(edges, []string{"A"}, map[string]bool{"N": true})
	if !reflect.DeepEqual(lost, []string{"D"}) {
		t.Fatalf("lost = %v, want [D]", lost)
	}
}

// Three independent AZs (A1->N1->D1, A2->N2->D2, A3->N3->D3), killing N1 (AZ 1) only
// loses D1 — A2/N2/D2 and A3/N3/D3 are untouched. This is the golden bundle's own
// clean-vs-broken distinction: independent per-AZ resources survive a single AZ kill.
func TestSimulateLoss_IndependentAZs_OnlyKilledAZsResourcesLost(t *testing.T) {
	edges := []DirectedEdge{
		{"entry", "N1"}, {"N1", "D1"},
		{"entry", "N2"}, {"N2", "D2"},
		{"entry", "N3"}, {"N3", "D3"},
	}
	lost := SimulateLoss(edges, []string{"entry"}, map[string]bool{"N1": true})
	if !reflect.DeepEqual(lost, []string{"D1"}) {
		t.Fatalf("lost = %v, want [D1] — killing AZ 1 must not affect AZ 2 or AZ 3", lost)
	}
}

// Redundant paths: entry -> N1 -> D and entry -> N2 -> D (two independent routes to
// the SAME destination). Killing N1 alone loses nothing — D is still reachable via N2.
func TestSimulateLoss_RedundantPaths_KillingOneSurvives(t *testing.T) {
	edges := []DirectedEdge{
		{"entry", "N1"}, {"N1", "D"},
		{"entry", "N2"}, {"N2", "D"},
	}
	lost := SimulateLoss(edges, []string{"entry"}, map[string]bool{"N1": true})
	if len(lost) != 0 {
		t.Fatalf("lost = %v, want empty — D survives via the redundant N2 path", lost)
	}
}

// Killing an entry point itself: everything only reachable through it is lost too.
func TestSimulateLoss_KillingEntryPoint_LosesEverythingBehindIt(t *testing.T) {
	edges := []DirectedEdge{{"entry", "X"}, {"X", "Y"}}
	lost := SimulateLoss(edges, []string{"entry"}, map[string]bool{"entry": true})
	if !reflect.DeepEqual(lost, []string{"X", "Y"}) {
		t.Fatalf("lost = %v, want [X Y]", lost)
	}
}

// A node directly killed is not ALSO reported as "lost due to disconnection" — killed
// and lost are different facts (see SimulateLoss's own doc comment).
func TestSimulateLoss_KilledNodeNotDoubleReportedAsLost(t *testing.T) {
	edges := []DirectedEdge{{"A", "N"}}
	lost := SimulateLoss(edges, []string{"A"}, map[string]bool{"N": true})
	for _, l := range lost {
		if l == "N" {
			t.Fatal("N was directly killed; it must not also appear in the lost list")
		}
	}
}

func TestSimulateLoss_NoKills_NothingLost(t *testing.T) {
	edges := []DirectedEdge{{"A", "B"}, {"B", "C"}}
	lost := SimulateLoss(edges, []string{"A"}, map[string]bool{})
	if len(lost) != 0 {
		t.Fatalf("lost = %v, want empty when nothing is killed", lost)
	}
}

// TestCascadeOrder_HopDistanceBeatsAlphabetical (PC-89) is the case that actually
// proves the fix matters: "Z" sorts before "B" alphabetically, but Z is one hop from
// the killed node and B is two — CascadeOrder must return [killed, Z, B], never
// [killed, B, Z]. Chain: killed -> Z -> B.
func TestCascadeOrder_HopDistanceBeatsAlphabetical(t *testing.T) {
	edges := []DirectedEdge{{"killed", "Z"}, {"Z", "B"}}
	members := map[string]bool{"killed": true, "Z": true, "B": true}
	got := CascadeOrder(edges, map[string]bool{"killed": true}, members)
	want := []string{"killed", "Z", "B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CascadeOrder = %v, want %v (hop distance, not alphabetical)", got, want)
	}
}

// TestCascadeOrder_TiesBrokenAlphabetically: two nodes at the SAME hop distance sort
// by ID — determinism needs a tiebreak, and alphabetical is what every other ordering
// in this codebase already uses when there's no other signal.
func TestCascadeOrder_TiesBrokenAlphabetically(t *testing.T) {
	edges := []DirectedEdge{{"killed", "Z"}, {"killed", "B"}}
	members := map[string]bool{"killed": true, "Z": true, "B": true}
	got := CascadeOrder(edges, map[string]bool{"killed": true}, members)
	want := []string{"killed", "B", "Z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CascadeOrder = %v, want %v (same hop distance, alphabetical tiebreak)", got, want)
	}
}

// TestCascadeOrder_UnreachedMemberSortsLast: a member with no forward path FROM any
// killed node (a disconnected fragment) must never be assigned a fabricated hop
// distance — it sorts after every real hop distance, not interleaved among them.
func TestCascadeOrder_UnreachedMemberSortsLast(t *testing.T) {
	edges := []DirectedEdge{{"killed", "Z"}} // "isolated" has no incoming edge from killed
	members := map[string]bool{"killed": true, "Z": true, "isolated": true}
	got := CascadeOrder(edges, map[string]bool{"killed": true}, members)
	want := []string{"killed", "Z", "isolated"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CascadeOrder = %v, want %v (unreached member last)", got, want)
	}
}
