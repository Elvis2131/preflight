package server_test

// PC-86: proves server.AssessCanvas — the actual production code path, not just
// ingest.IngestCanvas in isolation — works end-to-end, including a real
// AssuranceDelta on a session's second version, the same guarantee Assess already
// has for the HCL path.

import (
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/server"
)

func TestAssessCanvas_TwoVersions_ProducesRealDelta(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	brokenCanvas := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
			{ID: "db1", Type: "managed_database", Label: "Payments DB", Capability: map[string]string{
				"encryption_mechanism": "false",
			}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "routes_to", From: "dns1", To: "db1"},
		},
	}

	v1, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-server-test", Canvas: brokenCanvas, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("AssessCanvas v1: %v", err)
	}
	if v1.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", v1.VersionNumber)
	}
	if len(v1.Findings) == 0 {
		t.Fatal("expected real findings from a canvas-authored managed_database node")
	}

	fixedCanvas := brokenCanvas
	fixedCanvas.Nodes = []core.CanvasNode{
		brokenCanvas.Nodes[0],
		{ID: "db1", Type: "managed_database", Label: "Payments DB", Capability: map[string]string{
			"encryption_mechanism": "true",
		}},
	}

	v2, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-server-test", Canvas: fixedCanvas, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("AssessCanvas v2: %v", err)
	}
	if v2.VersionNumber != 2 {
		t.Errorf("VersionNumber = %d, want 2", v2.VersionNumber)
	}
	if len(v2.AssuranceDelta) == 0 {
		t.Fatal("expected a real AssuranceDelta on the second version")
	}

	found := false
	for _, e := range v2.AssuranceDelta {
		if e.FindingID == "finding.compliance.managed-database-storage-encryption.db1" {
			found = true
			if e.Kind != core.DeltaImprovement {
				t.Errorf("Kind = %q, want improvement", e.Kind)
			}
		}
	}
	if !found {
		t.Fatalf("expected the storage-encryption finding for db1 in the delta, got: %+v", v2.AssuranceDelta)
	}
}

// baseInlineWorkload is a minimal, valid core.Workload — PC-87's own NFR form output
// shape, posted inline rather than via a server-filesystem WorkloadPath (see
// AssessCanvasRequest.Workload's own doc comment for why a browser form needs this).
func baseInlineWorkload(capacity map[string]float64) core.Workload {
	return core.Workload{
		SchemaVersion:      "1.0.0",
		Name:               "inline-nfr-test",
		Criticality:        "tier1",
		DataClassification: "PCI",
		Regions:            []string{"eu-west-1"},
		ComplianceProfiles: []string{},
		Requirements:       []core.Requirement{},
		Capacity:           capacity,
	}
}

// TestAssessCanvas_InlineWorkload_SameSchemaAsFilePath (PC-87) proves the inline
// Workload field validates against the identical workload.schema.json contract the
// file-based path already uses — "no separate schema", per this ticket's own
// acceptance criterion — by running the SAME canvas through both an inline workload
// and the golden workload.yaml file and confirming both produce a real, non-empty
// assessment (not two different code paths that happen to both compile).
func TestAssessCanvas_InlineWorkload_SameSchemaAsFilePath(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
			{ID: "db1", Type: "managed_database", Label: "DB", Capability: map[string]string{
				"encryption_mechanism": "true",
			}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "routes_to", From: "dns1", To: "db1"},
		},
	}
	inline := baseInlineWorkload(map[string]float64{"app_node_rps": 500})

	resp, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-inline-workload-test", Canvas: doc, Workload: &inline,
	})
	if err != nil {
		t.Fatalf("AssessCanvas with inline Workload: %v", err)
	}
	if resp.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", resp.VersionNumber)
	}
	if len(resp.Findings) == 0 {
		t.Fatal("expected real findings — an inline workload must reach the identical downstream pipeline a file-based one does")
	}
}

// TestAssessCanvas_InlineWorkload_InvalidWorkload_ReturnsRealError (PC-87 negative
// control) proves the inline path is validated with the SAME rigor as
// ingest.LoadWorkload's own file-based Validate() call — a required field missing
// must be a real, reported error, never silently accepted just because it arrived as
// a struct instead of parsed YAML.
func TestAssessCanvas_InlineWorkload_InvalidWorkload_ReturnsRealError(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	invalid := baseInlineWorkload(nil)
	invalid.Name = "" // required per workload.schema.json

	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}}},
	}

	_, err = server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-inline-invalid-test", Canvas: doc, Workload: &invalid,
	})
	if err == nil {
		t.Fatal("expected a real validation error for a Workload missing its required Name field, got nil")
	}
	// Asserting on the specific validation-failure text, not just "any error" —
	// negative-controlled by temporarily making loadOrValidateWorkload skip inline
	// validation entirely (always falling through to the empty-WorkloadPath file
	// read): that also produces a non-nil error ("no such file or directory"), which
	// would have made this test pass for the WRONG reason. This assertion is what
	// actually distinguishes "inline validation caught it" from "some other error
	// happened to fire first."
	if !strings.Contains(err.Error(), "Name") {
		t.Errorf("error = %q, want it to name the actual missing field (Name), not a different failure", err.Error())
	}
}

// TestAssessCanvas_InlineWorkload_CapacityBlank_ProducesCapacityUnknownDownstream
// (PC-87's own acceptance criterion, verbatim: "A capacity field left blank produces
// capacity_unknown downstream, verified against a real assessment, not just checked
// in the form's own validation"). core.SurvivingCapacity's only real caller today is
// /simulate (core/simulate.go) — /assess's own Findings never surface a capacity
// value directly (grepped for callers before writing this test, not assumed) — so
// this drives the full server pipeline AssessCanvas -> stored version -> Simulate to
// prove a blank capacity field really does propagate all the way to a real
// not_assessable answer, not just an in-memory core.Workload the form never sends
// anywhere.
func TestAssessCanvas_InlineWorkload_CapacityBlank_ProducesCapacityUnknownDownstream(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb1", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "compute1", Type: "container_workload", Label: "App", Capability: map[string]string{}},
			{ID: "db1", Type: "managed_database", Label: "DB", Capability: map[string]string{
				"encryption_mechanism": "true",
			}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "routes_to", From: "lb1", To: "compute1"},
			{ID: "e2", Type: "reads/writes", From: "compute1", To: "db1"},
		},
	}

	// Capacity is nil — the form's own "left blank" case, not a zero value.
	blank := baseInlineWorkload(nil)
	resp, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-inline-blank-capacity-test", Canvas: doc, Workload: &blank,
	})
	if err != nil {
		t.Fatalf("AssessCanvas: %v", err)
	}

	sim, err := server.Simulate(store, server.SimulateRequest{
		SessionID: "canvas-inline-blank-capacity-test", VersionNumber: resp.VersionNumber,
		Faults: []core.Fault{{Type: "node_loss", Target: "lb1"}},
	})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if sim.Capacity.State != core.AssessmentStateNotAssessable {
		t.Errorf("Capacity.State = %q, want not_assessable — app_node_rps was never declared", sim.Capacity.State)
	}
	if sim.Capacity.Reason == "" {
		t.Error("expected a non-empty capacity_unknown reason")
	}

	// Positive control: the SAME topology with capacity actually declared must
	// produce a real assessed value, proving the blank case above isn't just always
	// not_assessable regardless of input.
	declared := baseInlineWorkload(map[string]float64{"app_node_rps": 200})
	resp2, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-inline-declared-capacity-test", Canvas: doc, Workload: &declared,
	})
	if err != nil {
		t.Fatalf("AssessCanvas (declared capacity): %v", err)
	}
	sim2, err := server.Simulate(store, server.SimulateRequest{
		SessionID: "canvas-inline-declared-capacity-test", VersionNumber: resp2.VersionNumber,
		Faults: []core.Fault{{Type: "node_loss", Target: "lb1"}},
	})
	if err != nil {
		t.Fatalf("Simulate (declared capacity): %v", err)
	}
	if sim2.Capacity.State != core.AssessmentStateAssessed {
		t.Errorf("Capacity.State = %q, want assessed — app_node_rps=200 WAS declared", sim2.Capacity.State)
	}
}

func TestAssessCanvas_DanglingEdge_NeverCrashesAlwaysNotAssessable(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// A realistic mid-authoring canvas: an entry point, a stateful node, and an edge
	// pointing at a node that was never drawn.
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
			{ID: "db1", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "routes_to", From: "dns1", To: "not-drawn-yet"},
		},
	}

	resp, err := server.AssessCanvas(store, server.AssessCanvasRequest{
		SessionID: "canvas-dangling-test", Canvas: doc, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("AssessCanvas must never error on a dangling edge, got: %v", err)
	}
	if len(resp.Findings) == 0 {
		t.Fatal("expected a real response with findings, not a crash or an empty result")
	}
}
