package ingest_test

// PC-86: hand-verified against small, constructed CanvasDocuments before trusting
// IngestCanvas against a real fixture captured from the actual running canvas app
// (canvas_golden_test.go covers that).

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

func TestIngestCanvas_TwoConnectedNodes_ProducesKnownEverything(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{"encryption_mechanism": "true"}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "routes_to", From: "lb", To: "db"},
		},
	}

	result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	if len(result.IR.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(result.IR.Nodes))
	}
	for _, n := range result.IR.Nodes {
		if n.Resolution != core.ResolutionKnown {
			t.Errorf("node %s: Resolution = %q, want known", n.ID, n.Resolution)
		}
	}
	if len(result.IR.Edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(result.IR.Edges))
	}
	if result.IR.Edges[0].Resolution != core.ResolutionKnown {
		t.Errorf("edge Resolution = %q, want known (both endpoints exist)", result.IR.Edges[0].Resolution)
	}

	dbNode := result.IR.Nodes[1]
	if dbNode.ID != "db" {
		// order isn't guaranteed by IngestCanvas's own contract, find it explicitly
		for _, n := range result.IR.Nodes {
			if n.ID == "db" {
				dbNode = n
			}
		}
	}
	if dbNode.Capability == nil || dbNode.Capability.EncryptionMechanism == nil || *dbNode.Capability.EncryptionMechanism != "true" {
		t.Errorf("db node Capability.EncryptionMechanism = %v, want \"true\"", dbNode.Capability)
	}
}

func TestIngestCanvas_DanglingEdge_IsUnresolvedNotDroppedNotError(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
		},
		Edges: []core.CanvasEdge{
			// "ghost" was never declared as a node — a dangling reference, the
			// steady state of using a canvas mid-authoring, per this ticket's own
			// Conversation.
			{ID: "e1", Type: "routes_to", From: "lb", To: "ghost"},
		},
	}

	result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	if len(result.IR.Edges) != 1 {
		t.Fatalf("got %d edges, want 1 — a dangling edge must never be silently dropped", len(result.IR.Edges))
	}
	edge := result.IR.Edges[0]
	if edge.Resolution != core.ResolutionUnresolved {
		t.Errorf("Resolution = %q, want unresolved", edge.Resolution)
	}
	if edge.Provenance.Reason == "" {
		t.Error("expected a non-empty Provenance.Reason explaining the dangling reference")
	}
}

func TestIngestCanvas_NoCapabilityDeclared_IsNilNotFabricated(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
		},
	}
	result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient == nil {
		t.Fatal("expected insufficient_model — one node, no entry point, this is a real MVG check, not a fixture-specific fluke")
	}
}

func TestIngestCanvas_EmptyCapabilityMap_ProducesNilCapabilityModel(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
		},
	}
	result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	for _, n := range result.IR.Nodes {
		if n.Capability != nil {
			t.Errorf("node %s: Capability = %+v, want nil — no capability was declared, never fabricate one", n.ID, n.Capability)
		}
	}
}
