package exporters_test

// PC-23: hand-verified against a small, constructed Finding+IR before trusting either
// exporter against the real golden fixture (exporters/golden_test.go covers that).

import (
	"encoding/json"
	"strings"
	"testing"

	"preflight/core"
	"preflight/exporters"
)

func syntheticZoneKillFinding() core.Finding {
	prov := core.NewProvenance(core.KindDerived, "test")
	return core.Finding{
		ID:    "finding.zone-kill.test-a",
		Title: "AZ loss: eu-test-1a test subnet",
		Dimensions: core.FailureMode{
			Trigger:            "availability zone loss",
			AffectedComponents: []string{"aws_subnet.test_a"},
			Detection:          core.DetectionModeled,
			Impact:             core.Assessed[any]("2 component(s) affected", prov).ToEnvelope(),
			Likelihood:         core.NotAssessable[any]("test", prov).ToEnvelope(),
			Detectability:      core.Assessed[any]("high", prov).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("test", prov).ToEnvelope(),
				RPOFeasible:        core.NotAssessable[any]("test", prov).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{{Description: "test"}},
		Outcome:  core.Assessed[any]("2 component(s) affected", prov).ToEnvelope(),
	}
}

func syntheticIRWithAZ(az string) *core.IR {
	return &core.IR{
		Nodes: []core.Node{
			{
				ID:            "aws_subnet.test_a",
				Type:          core.NodeTypeNetworkBoundary,
				Resolution:    core.ResolutionKnown,
				RawAttributes: map[string]any{"availability_zone": az},
				Provenance:    core.NewProvenance(core.KindStated, "test"),
			},
		},
	}
}

func TestExportFIS_ProducesValidJSONWithRealSchemaShape(t *testing.T) {
	finding := syntheticZoneKillFinding()
	ir := syntheticIRWithAZ("eu-test-1a")
	prov := core.NewProvenance(core.KindDerived, "test:export-fis")

	exp, err := exporters.ExportFIS(finding, ir, prov)
	if err != nil {
		t.Fatalf("ExportFIS: %v", err)
	}
	if exp.Format != "fis" {
		t.Errorf("Format = %q, want fis", exp.Format)
	}
	if exp.SourceFindingID != finding.ID {
		t.Errorf("SourceFindingID = %q, want %q", exp.SourceFindingID, finding.ID)
	}
	// Provenance must be the exporter's OWN, distinct from the finding's — PC-23's
	// own Conversation, verbatim.
	if exp.Provenance.IsZero() {
		t.Fatal("expected non-zero Provenance")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(exp.Content), &parsed); err != nil {
		t.Fatalf("Content is not valid JSON: %v", err)
	}
	for _, field := range []string{"description", "targets", "actions", "stopConditions", "roleArn", "tags"} {
		if _, ok := parsed[field]; !ok {
			t.Errorf("missing required FIS template field %q", field)
		}
	}

	targets := parsed["targets"].(map[string]any)
	target := targets["AffectedInstances"].(map[string]any)
	if target["resourceType"] != "aws:ec2:instance" {
		t.Errorf("resourceType = %v, want aws:ec2:instance", target["resourceType"])
	}
	filters := target["filters"].([]any)
	foundAZFilter := false
	for _, f := range filters {
		fm := f.(map[string]any)
		if fm["path"] == "Placement.AvailabilityZone" {
			foundAZFilter = true
			values := fm["values"].([]any)
			if len(values) != 1 || values[0] != "eu-test-1a" {
				t.Errorf("AZ filter values = %v, want [eu-test-1a]", values)
			}
		}
	}
	if !foundAZFilter {
		t.Error("expected a Placement.AvailabilityZone filter naming the finding's own affected AZ")
	}

	actions := parsed["actions"].(map[string]any)
	action := actions["StopInstances"].(map[string]any)
	if action["actionId"] != "aws:ec2:stop-instances" {
		t.Errorf("actionId = %v, want aws:ec2:stop-instances", action["actionId"])
	}
}

// TestExportFIS_NoAZAnywhere_ReturnsError covers the true worst case: no
// RawAttributes AND a Title that doesn't match the fallback convention either — the
// only situation this function cannot honestly resolve.
func TestExportFIS_NoAZAnywhere_ReturnsError(t *testing.T) {
	finding := syntheticZoneKillFinding()
	finding.Title = "some finding with no AZ mentioned at all"
	ir := &core.IR{Nodes: []core.Node{
		{ID: "aws_subnet.test_a", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown,
			Provenance: core.NewProvenance(core.KindStated, "test")}, // no RawAttributes at all
	}}
	prov := core.NewProvenance(core.KindDerived, "test")

	_, err := exporters.ExportFIS(finding, ir, prov)
	if err == nil {
		t.Fatal("expected an error — never fabricate a target AZ that was never stated anywhere")
	}
}

// TestExportFIS_NoRawAttribute_FallsBackToTitleConvention proves the documented
// fallback actually works — the exact real-world shape golden/aws hits (subnets use
// a locals reference the parser can't capture as a literal, but the Finding's own
// Title already states the AZ).
func TestExportFIS_NoRawAttribute_FallsBackToTitleConvention(t *testing.T) {
	finding := syntheticZoneKillFinding() // Title: "AZ loss: eu-test-1a test subnet"
	ir := &core.IR{Nodes: []core.Node{
		{ID: "aws_subnet.test_a", Type: core.NodeTypeNetworkBoundary, Resolution: core.ResolutionKnown,
			Provenance: core.NewProvenance(core.KindStated, "test")}, // no RawAttributes at all
	}}
	prov := core.NewProvenance(core.KindDerived, "test")

	exp, err := exporters.ExportFIS(finding, ir, prov)
	if err != nil {
		t.Fatalf("expected the Title fallback to succeed: %v", err)
	}
	if !strings.Contains(exp.Content, "eu-test-1a") {
		t.Errorf("expected the fallback-derived AZ (eu-test-1a) in the export, got:\n%s", exp.Content)
	}
}

func TestExportLitmus_ProducesValidYAMLWithRealSchemaShape(t *testing.T) {
	finding := syntheticZoneKillFinding()
	ir := syntheticIRWithAZ("eu-test-1a")
	prov := core.NewProvenance(core.KindDerived, "test:export-litmus")

	exp, err := exporters.ExportLitmus(finding, ir, prov)
	if err != nil {
		t.Fatalf("ExportLitmus: %v", err)
	}
	if exp.Format != "litmus" {
		t.Errorf("Format = %q, want litmus", exp.Format)
	}

	for _, want := range []string{
		"apiVersion: litmuschaos.io/v1alpha1",
		"kind: ChaosEngine",
		"name: node-drain",
		"NODE_LABEL",
		"topology.kubernetes.io/zone=eu-test-1a",
	} {
		if !strings.Contains(exp.Content, want) {
			t.Errorf("Content missing expected substring %q\n---\n%s", want, exp.Content)
		}
	}
}

func TestExportLitmus_And_ExportFIS_HaveDistinctProvenanceFromEachOther(t *testing.T) {
	finding := syntheticZoneKillFinding()
	ir := syntheticIRWithAZ("eu-test-1a")

	fisProv := core.NewProvenance(core.KindDerived, "test:export-fis")
	litmusProv := core.NewProvenance(core.KindDerived, "test:export-litmus")

	fisExp, err := exporters.ExportFIS(finding, ir, fisProv)
	if err != nil {
		t.Fatal(err)
	}
	litmusExp, err := exporters.ExportLitmus(finding, ir, litmusProv)
	if err != nil {
		t.Fatal(err)
	}
	if fisExp.Provenance.Source == litmusExp.Provenance.Source {
		t.Error("expected FIS and Litmus exports to carry distinct provenance sources, not the same one copy-pasted")
	}
}
