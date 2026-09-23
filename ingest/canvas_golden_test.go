package ingest_test

// PC-86's own round-trip fixture test, per the explicit instruction that prompted
// building the canvas.schema.json contract in the first place: "a round-trip fixture
// test in PC-86 itself (serialize a known canvas → assert the Go parser produces a
// specific, hand-verified IR) — same golden-fixture discipline ingest/ already has,
// applied to the second producer now that there is one."
//
// The fixture used here (contracts/samples/canvas.sample.json) is not hand-typed —
// it's the exact JSON captured from actually driving the real canvas/ app in a live
// browser (Playwright), the same discipline "derive, then hand-verify" that
// golden/fixtures/*.json already follows for the HCL side.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/ingest"
)

func loadCanvasSample(t *testing.T) core.CanvasDocument {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(wd, "..", "contracts", "samples", "canvas.sample.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc core.CanvasDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("sample failed its own contract's Validate(): %v", err)
	}
	return doc
}

// TestIngestCanvas_RoundTrip_AgainstTheRealCapturedSample is the round-trip test: a
// known, real (not hand-typed) canvas document produces a specific, hand-verified IR
// — the same load-bearing guarantee golden/fixtures/*.ir.json already proves for the
// HCL producer, now proven for the second one.
func TestIngestCanvas_RoundTrip_AgainstTheRealCapturedSample(t *testing.T) {
	doc := loadCanvasSample(t)

	// Hand-verified against contracts/samples/canvas.sample.json's own real content,
	// read directly: 2 nodes (load_balancer-1, managed_database-2), 1 edge between
	// them (reads/writes), the database node carrying encryption_mechanism=true.
	if len(doc.Nodes) != 2 || len(doc.Edges) != 1 {
		t.Fatalf("precondition failed: sample has %d nodes / %d edges, want 2/1 — did the sample file change?", len(doc.Nodes), len(doc.Edges))
	}

	result, err := ingest.IngestCanvas(doc, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}

	if len(result.IR.Nodes) != 2 {
		t.Fatalf("got %d IR nodes, want 2", len(result.IR.Nodes))
	}
	if len(result.IR.Edges) != 1 {
		t.Fatalf("got %d IR edges, want 1", len(result.IR.Edges))
	}

	var lb, db *core.Node
	for i := range result.IR.Nodes {
		n := &result.IR.Nodes[i]
		switch n.ID {
		case "load_balancer-1":
			lb = n
		case "managed_database-2":
			db = n
		}
	}
	if lb == nil {
		t.Fatal("expected node load_balancer-1 in the IR")
	}
	if lb.Type != core.NodeTypeLoadBalancer {
		t.Errorf("load_balancer-1: Type = %q, want load_balancer", lb.Type)
	}
	if lb.Resolution != core.ResolutionKnown {
		t.Errorf("load_balancer-1: Resolution = %q, want known", lb.Resolution)
	}

	if db == nil {
		t.Fatal("expected node managed_database-2 in the IR")
	}
	if db.Type != core.NodeTypeManagedDatabase {
		t.Errorf("managed_database-2: Type = %q, want managed_database", db.Type)
	}
	if db.Capability == nil || db.Capability.EncryptionMechanism == nil || *db.Capability.EncryptionMechanism != "true" {
		t.Errorf("managed_database-2: Capability.EncryptionMechanism = %v, want \"true\" (matches the real sample's own declared capability)", db.Capability)
	}

	edge := result.IR.Edges[0]
	if edge.Type != core.EdgeTypeReadsWrites {
		t.Errorf("edge Type = %q, want reads/writes", edge.Type)
	}
	if edge.From != "load_balancer-1" || edge.To != "managed_database-2" {
		t.Errorf("edge From/To = %q/%q, want load_balancer-1/managed_database-2", edge.From, edge.To)
	}
	if edge.Resolution != core.ResolutionKnown {
		t.Errorf("edge Resolution = %q, want known (both endpoints exist in this same document)", edge.Resolution)
	}

	// This is what makes it a genuine round-trip proof, not just a parse check: the
	// resulting IR must itself pass core.IR's own contract validation — the same
	// frozen ir.schema.json every HCL-produced IR must satisfy. No canvas-specific
	// relaxation exists.
	if err := result.IR.Validate(); err != nil {
		t.Errorf("the IR produced from a canvas document failed core.IR's own Validate(): %v", err)
	}
}
