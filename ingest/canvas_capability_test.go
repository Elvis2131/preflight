package ingest_test

// PC-136: canvas-authored nodes resolve a real PC-107 capability_level from
// CanvasNode.ServiceID through the exact same providers.Registry.Lookup the
// Terraform ingest path already uses — found missing during PC-127 live
// verification, where every canvas-built journey was blocked at its first hop
// because no canvas node ever carried a capability_level at all.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

// canvasDBWithEntryPoint pairs the ServiceID-under-test managed_database node with a
// plain load_balancer entry point node — CheckMVG (ingest/mvg.go) requires both an
// entry point and a stateful node before it will produce a real IR at all, so a
// single-node canvas document (correct as far as PC-136's own concern goes) would
// otherwise fail these tests for an unrelated reason.
func canvasDBWithEntryPoint(dbServiceID string) core.CanvasDocument {
	return core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, ServiceID: dbServiceID},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "db"},
		},
	}
}

func dbNode(t *testing.T, result ingest.Result) core.Node {
	t.Helper()
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	for _, n := range result.IR.Nodes {
		if n.ID == "db" {
			return n
		}
	}
	t.Fatal("no node with ID \"db\" in result")
	return core.Node{}
}

func TestIngestCanvas_ServiceID_ResolvesRealCapabilityLevel(t *testing.T) {
	reg := loadRegistry(t)
	doc := canvasDBWithEntryPoint("aws_db_instance")

	result, err := ingest.IngestCanvas(doc, providers.Registry(reg), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	got := dbNode(t, result).RawAttributes["capability_level"]
	if got != string(core.CapabilityFailureSimulation) {
		t.Errorf("capability_level = %v, want %q (aws_db_instance's real registry entry)", got, core.CapabilityFailureSimulation)
	}
}

func TestIngestCanvas_ServiceID_MissingOrUnknownLeavesCapabilityLevelUnresolved(t *testing.T) {
	reg := loadRegistry(t)
	tests := []struct {
		name      string
		serviceID string
	}{
		{"empty ServiceID (no service selected)", ""},
		{"unknown ServiceID (no registry entry)", "aws_does_not_exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := canvasDBWithEntryPoint(tt.serviceID)
			result, err := ingest.IngestCanvas(doc, providers.Registry(reg), 1)
			if err != nil {
				t.Fatalf("IngestCanvas: %v", err)
			}
			db := dbNode(t, result)
			if v, ok := db.RawAttributes["capability_level"]; ok {
				t.Errorf("capability_level = %v, want absent (I4: never guessed)", v)
			}
		})
	}
}

// TestIngestCanvas_ServiceID_MismatchedNodeType_LeavesCapabilityLevelUnresolved is the
// case a naive implementation (trusting ServiceID alone, or trusting the node's own
// declared Type alone) would get wrong: a ServiceID borrowed from a different
// structural category must never be trusted just because SOME registry entry exists
// for that string.
func TestIngestCanvas_ServiceID_MismatchedNodeType_LeavesCapabilityLevelUnresolved(t *testing.T) {
	reg := loadRegistry(t)
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			// aws_db_instance's real registry entry declares node_type: managed_database —
			// declaring this canvas node as compute is an inconsistency this function must
			// refuse to resolve, not silently pick a side of.
			{ID: "mismatched", Type: "compute", Label: "Mismatched", Capability: map[string]string{}, ServiceID: "aws_db_instance"},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "db"},
		},
	}
	result, err := ingest.IngestCanvas(doc, providers.Registry(reg), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	for _, n := range result.IR.Nodes {
		if n.ID != "mismatched" {
			continue
		}
		if v, ok := n.RawAttributes["capability_level"]; ok {
			t.Errorf("capability_level = %v, want absent for a NodeType/ServiceID mismatch", v)
		}
		return
	}
	t.Fatal("no node with ID \"mismatched\" in result")
}

// TestParity_CanvasVsTerraform_SameServiceYieldsIdenticalCapabilityLevel is PC-136's
// own explicit acceptance criterion: "the same architecture built on canvas and
// loaded from Terraform must produce the same capability levels per node." Built
// against ingest/testdata/sufficient-fixture (an existing, real Terraform fixture —
// aws_lb + aws_db_instance), not a fixture invented just for this test, and against
// the SAME real registry both paths would use in production (awsprovider.Load()).
func TestParity_CanvasVsTerraform_SameServiceYieldsIdenticalCapabilityLevel(t *testing.T) {
	reg := loadRegistry(t)

	tfResult, err := ingest.Ingest("testdata/sufficient-fixture", reg, 1)
	if err != nil {
		t.Fatalf("Ingest (Terraform path): %v", err)
	}
	if tfResult.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", tfResult.Insufficient)
	}

	canvasDoc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}, ServiceID: "aws_lb"},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, ServiceID: "aws_db_instance"},
		},
		Edges: []core.CanvasEdge{
			{ID: "e1", Type: "depends_on", From: "lb", To: "db"},
		},
	}
	canvasResult, err := ingest.IngestCanvas(canvasDoc, providers.Registry(reg), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if canvasResult.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", canvasResult.Insufficient)
	}

	tfLevelByType := map[core.NodeType]any{}
	for _, n := range tfResult.IR.Nodes {
		tfLevelByType[n.Type] = n.RawAttributes["capability_level"]
	}
	canvasLevelByType := map[core.NodeType]any{}
	for _, n := range canvasResult.IR.Nodes {
		canvasLevelByType[n.Type] = n.RawAttributes["capability_level"]
	}

	for nodeType, tfLevel := range tfLevelByType {
		canvasLevel, ok := canvasLevelByType[nodeType]
		if !ok {
			t.Errorf("node_type %s: present in Terraform result, missing from canvas result", nodeType)
			continue
		}
		if tfLevel != canvasLevel {
			t.Errorf("node_type %s: Terraform capability_level = %v, canvas capability_level = %v — parity broken", nodeType, tfLevel, canvasLevel)
		}
		if tfLevel == nil {
			t.Errorf("node_type %s: both paths report nil capability_level — this parity check is vacuous unless at least one node_type resolves a real level", nodeType)
		}
	}
}

// PC-156: aws_instance is mapped for HCL ingest only (the canvas cannot author a launch-time
// public-address setting or an Elastic IP). A hand-built canvas node naming that service must
// stay unmapped, never claiming a capability the canvas cannot back.
func TestIngestCanvas_IngestOnlyService_IsNotResolved(t *testing.T) {
	reg := loadRegistry(t)
	if m, ok := reg["aws_instance"]; !ok || !m.IngestOnly {
		t.Fatal("test premise: aws_instance must be an ingest-only mapping")
	}
	doc := core.CanvasDocument{Nodes: []core.CanvasNode{
		{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
		{ID: "ec2", Type: "compute", ServiceID: "aws_instance", Label: "EC2", Capability: map[string]string{}},
		{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}},
	}}
	result, err := ingest.IngestCanvas(doc, providers.Registry(reg), 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	for _, n := range result.IR.Nodes {
		if n.ID == "ec2" {
			if v, ok := n.RawAttributes["capability_level"]; ok {
				t.Errorf("capability_level = %v, want absent for an ingest-only service", v)
			}
			return
		}
	}
	t.Fatal("no node ec2")
}
