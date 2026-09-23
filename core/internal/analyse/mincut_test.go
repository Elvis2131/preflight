package analyse

import (
	"reflect"
	"testing"
)

// This file exercises MinVertexCut ONLY against small, hand-drawn graphs with a
// hand-computed expected answer — no core.IR, no golden fixture, nothing from the real
// pipeline. Per the explicit sequencing requirement before this work started: verify
// the algorithm is correct in isolation first, so a bug here is a five-minute fix
// against a graph small enough to check by inspection, not a debugging session inside
// a full simulation trace.

// A -> B -> D
// A -> C -> D
// Two vertex-disjoint paths of length 2. Removing either B or C alone leaves the other
// path intact, so BOTH must be removed to disconnect A from D. Hand-computed min cut:
// size 2, vertices {B, C}.
func TestMinVertexCut_Diamond_SizeTwo(t *testing.T) {
	edges := []DirectedEdge{
		{"A", "B"}, {"B", "D"},
		{"A", "C"}, {"C", "D"},
	}
	got := MinVertexCut(edges, "A", "D")
	if got.Size != 2 {
		t.Fatalf("Size = %d, want 2", got.Size)
	}
	if !reflect.DeepEqual(got.CutVertices, []string{"B", "C"}) {
		t.Fatalf("CutVertices = %v, want [B C]", got.CutVertices)
	}
}

// A -> B -> C -> D. A single chain: removing B (or C) disconnects A from D. Hand-
// computed min cut: size 1. Either {B} or {C} is a VALID answer (both are genuine
// single-vertex cuts) — this test asserts Size and that the result IS one of the two
// valid single-vertex cuts, not a specific one, since MinVertexCut's own contract
// (see its doc comment) promises reproducibility, not a specific canonical choice
// among ties.
func TestMinVertexCut_Chain_SizeOne(t *testing.T) {
	edges := []DirectedEdge{
		{"A", "B"}, {"B", "C"}, {"C", "D"},
	}
	got := MinVertexCut(edges, "A", "D")
	if got.Size != 1 {
		t.Fatalf("Size = %d, want 1", got.Size)
	}
	if len(got.CutVertices) != 1 {
		t.Fatalf("CutVertices = %v, want exactly one vertex", got.CutVertices)
	}
	v := got.CutVertices[0]
	if v != "B" && v != "C" {
		t.Fatalf("CutVertices = %v, want {B} or {C}", got.CutVertices)
	}
}

// A -> N -> D (single chokepoint), matching the golden bundle's own single-NAT-gateway
// SPOF shape (golden/aws-broken/network.tf's defect 1: one NAT gateway serves three
// private subnets). Hand-computed min cut: size 1, vertex {N}.
func TestMinVertexCut_SingleChokepoint_MatchesNATGatewayShape(t *testing.T) {
	edges := []DirectedEdge{
		{"private_subnet_a", "N"}, {"private_subnet_b", "N"}, {"private_subnet_c", "N"},
		{"N", "internet"},
	}
	got := MinVertexCut(edges, "private_subnet_a", "internet")
	if got.Size != 1 {
		t.Fatalf("Size = %d, want 1", got.Size)
	}
	if !reflect.DeepEqual(got.CutVertices, []string{"N"}) {
		t.Fatalf("CutVertices = %v, want [N]", got.CutVertices)
	}
}

// Three fully independent NAT gateways, one per AZ — matching golden/aws (the CLEAN
// bundle)'s actual topology, no shared chokepoint. Hand-computed min cut between the
// AZ-a source and the internet: size 1 still (N1 alone, on the ONE path this specific
// source uses) — a single source only ever has one path unless the graph itself
// fans back in. This test exists to prove the DIFFERENCE from the broken shape isn't
// in this source's own cut value, but in whether OTHER sources share N1 — see the
// next test for the actual redundancy proof.
func TestMinVertexCut_IndependentPerAZChokepoint_StillSizeOnePerSource(t *testing.T) {
	edges := []DirectedEdge{
		{"private_subnet_a", "N1"}, {"N1", "internet"},
		{"private_subnet_b", "N2"}, {"N2", "internet"},
		{"private_subnet_c", "N3"}, {"N3", "internet"},
	}
	got := MinVertexCut(edges, "private_subnet_a", "internet")
	if got.Size != 1 || !reflect.DeepEqual(got.CutVertices, []string{"N1"}) {
		t.Fatalf("got %+v, want Size=1 CutVertices=[N1] — subnet_a's OWN path still has exactly one gateway on it", got)
	}
}

// Genuinely redundant paths: TWO independent gateways both serving the SAME subnet
// (unlike the single-chokepoint case above, where subnet_a has only one path at all).
// Hand-computed min cut: size 2, vertices {N1, N2} — an actually resilient egress
// design, correctly NOT reported as a single point of failure.
func TestMinVertexCut_RedundantGateways_SizeTwo(t *testing.T) {
	edges := []DirectedEdge{
		{"private_subnet_a", "N1"}, {"N1", "internet"},
		{"private_subnet_a", "N2"}, {"N2", "internet"},
	}
	got := MinVertexCut(edges, "private_subnet_a", "internet")
	if got.Size != 2 {
		t.Fatalf("Size = %d, want 2 (two independent gateways = not a SPOF)", got.Size)
	}
	if !reflect.DeepEqual(got.CutVertices, []string{"N1", "N2"}) {
		t.Fatalf("CutVertices = %v, want [N1 N2]", got.CutVertices)
	}
}

// No path from source to sink at all: vacuously safe, not a SPOF. Hand-computed:
// size 0, empty cut.
func TestMinVertexCut_NoPath_SizeZero(t *testing.T) {
	edges := []DirectedEdge{
		{"A", "B"}, // C is disconnected from this component entirely
		{"X", "Y"},
	}
	got := MinVertexCut(edges, "A", "Y")
	if got.Size != 0 {
		t.Fatalf("Size = %d, want 0 (no dependency path exists)", got.Size)
	}
	if len(got.CutVertices) != 0 {
		t.Fatalf("CutVertices = %v, want empty", got.CutVertices)
	}
}

// A node is never its own SPOF.
func TestMinVertexCut_SourceEqualsSink(t *testing.T) {
	got := MinVertexCut([]DirectedEdge{{"A", "B"}}, "A", "A")
	if got.Size != 0 || len(got.CutVertices) != 0 {
		t.Fatalf("got %+v, want zero value", got)
	}
}

// Direct edge, no intermediate vertex at all: UNCUTTABLE, not "Size 0" — a direct
// dependency has no third-party vertex whose removal could ever disconnect it. Size 0
// is reserved for "no path exists at all" (TestMinVertexCut_NoPath_SizeZero); this is
// the opposite situation (a path exists and can never be severed by vertex removal),
// and conflating the two was the actual bug this test file caught before the algorithm
// ever touched real IR data — see mincut.go's MinVertexCutResult doc comment.
func TestMinVertexCut_DirectEdge_Uncuttable(t *testing.T) {
	got := MinVertexCut([]DirectedEdge{{"A", "B"}}, "A", "B")
	if !got.Uncuttable {
		t.Fatalf("got %+v, want Uncuttable=true", got)
	}
	if got.Size != 0 || len(got.CutVertices) != 0 {
		t.Fatalf("got %+v, want Size=0 and empty CutVertices alongside Uncuttable=true", got)
	}
}

// A direct bypass edge coexisting with a real intermediate-vertex path: still
// uncuttable overall (the bypass alone guarantees connectivity no matter what happens
// to X), even though X is a perfectly real vertex on ONE of the two paths. Confirms
// the fix checks for a literal direct edge specifically, not merely "graph has more
// than one path" or "graph has any intermediate vertex at all."
func TestMinVertexCut_DirectBypassAlongsideRealVertex_StillUncuttable(t *testing.T) {
	got := MinVertexCut([]DirectedEdge{{"A", "X"}, {"X", "B"}, {"A", "B"}}, "A", "B")
	if !got.Uncuttable {
		t.Fatalf("got %+v, want Uncuttable=true (the direct A->B bypass makes X irrelevant)", got)
	}
}

// The negative control: a genuine two-vertex path with NO direct bypass must still be
// correctly reported as a normal, cuttable size-1 SPOF — proving the direct-edge check
// added above didn't overreach and start treating ordinary two-hop paths as uncuttable.
func TestMinVertexCut_TwoHopPath_StillNormallyCuttable(t *testing.T) {
	got := MinVertexCut([]DirectedEdge{{"A", "X"}, {"X", "B"}}, "A", "B")
	if got.Uncuttable {
		t.Fatalf("got %+v, want Uncuttable=false — there is no direct A->B edge here", got)
	}
	if got.Size != 1 || !reflect.DeepEqual(got.CutVertices, []string{"X"}) {
		t.Fatalf("got %+v, want Size=1 CutVertices=[X]", got)
	}
}

// Determinism (I1): the SAME graph run twice must produce the EXACT same result,
// including which specific vertex is reported when several equal-size cuts exist
// (TestMinVertexCut_Chain_SizeOne already proves EITHER is individually valid; this
// test proves the algorithm doesn't flip between them across runs).
func TestMinVertexCut_Deterministic_AcrossRepeatedRuns(t *testing.T) {
	edges := []DirectedEdge{
		{"A", "B"}, {"B", "C"}, {"C", "D"},
		{"A", "E"}, {"E", "F"}, {"F", "D"},
	}
	first := MinVertexCut(edges, "A", "D")
	for i := 0; i < 20; i++ {
		got := MinVertexCut(edges, "A", "D")
		if got.Size != first.Size || !reflect.DeepEqual(got.CutVertices, first.CutVertices) {
			t.Fatalf("run %d: got %+v, want identical to first run %+v — MinVertexCut must be deterministic", i, got, first)
		}
	}
}
