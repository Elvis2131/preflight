package exporters_test

// PC-27's acceptance criteria, hand-verified:
//   - "Exported CALM JSON validates against the official CALM JSON Schema in CI" —
//     TestExportCALM_AgainstGoldenAWSBundle_ValidatesAgainstOfficialSchema, run against
//     the real golden bundle, and TestValidateCALM_RejectsMalformedExport, the
//     deliberate-malformed-input check the Card's own Conversation asks for by name
//     ("confirm it actually catches problems, not just that it passes on the happy
//     path").
//   - "The export is documented as one-way and lossy ... directly in the export
//     output" — TestExportCALM_StatesLossyNoticeInOutput.
//   - "No changes to core IR were required" — satisfied by construction (ExportCALM
//     only reads core.IR/core.Node/core.Edge as PC-11 already defined them); nothing
//     to test here beyond core/'s own existing test suite staying green, which the
//     project's full verification pass already confirms.

import (
	"encoding/json"
	"strings"
	"testing"

	"preflight/core"
	"preflight/exporters"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestExportCALM_AgainstGoldenAWSBundle_ValidatesAgainstOfficialSchema(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	content, err := exporters.ExportCALM(result.IR)
	if err != nil {
		t.Fatalf("ExportCALM against the real golden bundle: %v", err)
	}

	// ExportCALM already self-validates before returning; re-validating here proves
	// the check is real (not a rubber stamp) by running the exact same official-schema
	// validation this test's own name promises, independently of ExportCALM's internals.
	if err := exporters.ValidateCALM(content); err != nil {
		t.Fatalf("golden bundle's CALM export failed official-schema validation: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		t.Fatalf("CALM export is not valid JSON: %v", err)
	}
	nodes, ok := doc["nodes"].([]any)
	if !ok || len(nodes) != len(result.IR.Nodes) {
		t.Fatalf("got %d CALM nodes, want %d (one per IR node)", len(nodes), len(result.IR.Nodes))
	}
	rels, ok := doc["relationships"].([]any)
	if !ok || len(rels) != len(result.IR.Edges) {
		t.Fatalf("got %d CALM relationships, want %d (one per IR edge)", len(rels), len(result.IR.Edges))
	}
}

// TestValidateCALM_RejectsMalformedExport is PC-27's first acceptance criterion's own
// explicit instruction, verbatim: "deliberately test CI validation against the
// official CALM JSON Schema with a malformed export to confirm it actually catches
// problems, not just that it passes on the happy path." A node missing its required
// "description" field is real malformation the vendored official schema (core.json's
// own "required": ["unique-id", "node-type", "name", "description"]) is confirmed to
// reject — checked directly against the schema, not assumed.
func TestValidateCALM_RejectsMalformedExport(t *testing.T) {
	malformed := []byte(`{
		"nodes": [{"unique-id": "n1", "node-type": "database", "name": "n1"}],
		"relationships": []
	}`)
	if err := exporters.ValidateCALM(malformed); err == nil {
		t.Fatal("expected a malformed CALM document (node missing required \"description\") to fail official-schema validation, got nil")
	}
}

func TestValidateCALM_AcceptsMinimalWellFormedDocument(t *testing.T) {
	valid := []byte(`{
		"nodes": [{"unique-id": "n1", "node-type": "database", "name": "n1", "description": "d1"}],
		"relationships": [],
		"metadata": {}
	}`)
	if err := exporters.ValidateCALM(valid); err != nil {
		t.Fatalf("expected a well-formed minimal CALM document to pass, got: %v", err)
	}
}

func TestExportCALM_StatesLossyNoticeInOutput(t *testing.T) {
	ir := &core.IR{
		SchemaVersion: "1.0.0",
		VersionNumber: 1,
		VersionHash:   "sha256:test",
		Nodes: []core.Node{
			{
				ID:         "n1",
				Type:       core.NodeTypeManagedDatabase,
				Resolution: core.ResolutionKnown,
				Provenance: core.NewProvenance(core.KindStated, "test.tf:1:n1"),
			},
		},
	}
	content, err := exporters.ExportCALM(ir)
	if err != nil {
		t.Fatalf("ExportCALM: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		t.Fatalf("CALM export is not valid JSON: %v", err)
	}
	metadata, ok := doc["metadata"].(map[string]any)
	if !ok {
		t.Fatal("CALM export has no top-level metadata object")
	}
	notice, ok := metadata["notice"].(string)
	if !ok || notice == "" {
		t.Fatal("CALM export's metadata has no non-empty \"notice\" field")
	}
	for _, must := range []string{"one-way", "lossy", "NOT round-trippable"} {
		if !strings.Contains(notice, must) {
			t.Errorf("notice = %q, want it to state %q", notice, must)
		}
	}
}

func TestExportCALM_ContainedInEdgeBecomesDeployedIn(t *testing.T) {
	ir := &core.IR{
		SchemaVersion: "1.0.0", VersionNumber: 1, VersionHash: "sha256:test",
		Nodes: []core.Node{
			{ID: "child", Type: core.NodeTypeCompute, Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "t:1:child")},
			{ID: "parent", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "t:2:parent")},
		},
		Edges: []core.Edge{
			{ID: "e1", Type: core.EdgeTypeContainedIn, From: "child", To: "parent", Resolution: core.ResolutionKnown, Provenance: core.NewProvenance(core.KindStated, "t:1:child")},
		},
	}
	content, err := exporters.ExportCALM(ir)
	if err != nil {
		t.Fatalf("ExportCALM: %v", err)
	}
	var doc map[string]any
	json.Unmarshal(content, &doc)
	rels := doc["relationships"].([]any)
	if len(rels) != 1 {
		t.Fatalf("got %d relationships, want 1", len(rels))
	}
	rel := rels[0].(map[string]any)
	relType := rel["relationship-type"].(map[string]any)
	deployedIn, ok := relType["deployed-in"].(map[string]any)
	if !ok {
		t.Fatalf("relationship-type = %v, want a deployed-in relationship for a contained_in edge", relType)
	}
	if deployedIn["container"] != "parent" {
		t.Errorf("container = %v, want %q", deployedIn["container"], "parent")
	}
	nodes := deployedIn["nodes"].([]any)
	if len(nodes) != 1 || nodes[0] != "child" {
		t.Errorf("nodes = %v, want [child]", nodes)
	}
}
