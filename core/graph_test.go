package core_test

// PC-81: hand-verified against a small, constructed IR before trusting RenderDOT
// against a real golden bundle (render/render_golden_test.go covers the real
// Graphviz subprocess round trip) — the same discipline every other engine in this
// codebase established.

import (
	"strings"
	"testing"

	"preflight/core"
)

func smallIR() *core.IR {
	return &core.IR{
		SchemaVersion: "1.0.0", VersionNumber: 1, VersionHash: "test",
		Nodes: []core.Node{
			{ID: "z-node", Type: core.NodeTypeCache, Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test")},
			{ID: "a-node", Type: core.NodeTypeDNS, Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test")},
			{ID: "unresolved-node", Type: core.NodeTypeManagedDatabase, Resolution: core.ResolutionUnresolved, Provenance: core.NewProvenance(core.KindStated, "test")},
		},
		Edges: []core.Edge{
			{ID: "e2", Type: core.EdgeTypeRoutesTo, From: "z-node", To: "a-node", Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test")},
			{ID: "e1", Type: core.EdgeTypeReadsWrites, From: "a-node", To: "z-node", Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test")},
		},
	}
}

func TestRenderDOT_SortsNodesByID_NotInputOrder(t *testing.T) {
	dot := core.RenderDOT(smallIR())
	aIdx := strings.Index(dot, `"a-node"`)
	zIdx := strings.Index(dot, `"z-node"`)
	if aIdx == -1 || zIdx == -1 {
		t.Fatalf("expected both node IDs quoted in output, got:\n%s", dot)
	}
	if aIdx >= zIdx {
		t.Errorf("a-node (idx %d) should appear before z-node (idx %d) — nodes must be sorted by ID, not input order", aIdx, zIdx)
	}
}

func TestRenderDOT_UnresolvedNodeGetsDashedStyle(t *testing.T) {
	dot := core.RenderDOT(smallIR())
	lines := strings.Split(dot, "\n")
	var unresolvedLine string
	for _, l := range lines {
		if strings.Contains(l, "unresolved-node") {
			unresolvedLine = l
		}
	}
	if unresolvedLine == "" {
		t.Fatal("expected a line for unresolved-node")
	}
	if !strings.Contains(unresolvedLine, "dashed") {
		t.Errorf("unresolved node line = %q, want it to contain \"dashed\"", unresolvedLine)
	}
}

func TestRenderDOT_NodeIDsAreQuoted_HandlesRealResourceAddressCharacters(t *testing.T) {
	ir := &core.IR{
		Nodes: []core.Node{
			{ID: "aws_db_instance.payments", Type: core.NodeTypeManagedDatabase, Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "test")},
		},
	}
	dot := core.RenderDOT(ir)
	if !strings.Contains(dot, `"aws_db_instance.payments"`) {
		t.Errorf("expected the real dotted resource address quoted verbatim, got:\n%s", dot)
	}
}

func TestRenderDOT_Deterministic_SameIRTwiceProducesByteIdenticalOutput(t *testing.T) {
	ir := smallIR()
	first := core.RenderDOT(ir)
	second := core.RenderDOT(ir)
	if first != second {
		t.Fatalf("RenderDOT produced different output for the identical IR twice:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
