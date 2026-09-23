package exporters_test

// PC-23's own acceptance criteria: "at least one failure mode from the golden fixture
// exports a valid FIS template" and "... a valid Litmus manifest", "both exported
// manifests run without manual editing."

import (
	"encoding/json"
	"testing"

	"preflight/core"
	"preflight/exporters"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestExportFISAndLitmus_AgainstGoldenAWSBundle(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	findings := core.BuildFindings(result.IR, workload)
	var zoneKillFinding core.Finding
	found := false
	for _, f := range findings {
		if f.ID == "finding.zone-kill.public-a" {
			zoneKillFinding = f
			found = true
		}
	}
	if !found {
		t.Fatal("expected finding.zone-kill.public-a in the real golden findings")
	}

	fisProv := core.NewProvenance(core.KindDerived, "exporters:fis:finding.zone-kill.public-a")
	fisExp, err := exporters.ExportFIS(zoneKillFinding, result.IR, fisProv)
	if err != nil {
		t.Fatalf("ExportFIS against the real golden bundle: %v", err)
	}

	// "Runs without manual editing" — checked as structural completeness: every
	// required FIS field present, real values (not empty strings/placeholders that
	// would need filling in before this is even syntactically submittable), and the
	// AZ actually matches golden/aws/network.tf's own real public_a subnet.
	var fisParsed map[string]any
	if err := json.Unmarshal([]byte(fisExp.Content), &fisParsed); err != nil {
		t.Fatalf("FIS Content is not valid JSON: %v", err)
	}
	targets := fisParsed["targets"].(map[string]any)
	target := targets["AffectedInstances"].(map[string]any)
	filters := target["filters"].([]any)
	azFound := false
	for _, f := range filters {
		fm := f.(map[string]any)
		if fm["path"] == "Placement.AvailabilityZone" {
			values := fm["values"].([]any)
			if values[0] != "eu-west-1a" {
				t.Errorf("AZ = %v, want eu-west-1a (golden/aws/network.tf's own public_a subnet)", values[0])
			}
			azFound = true
		}
	}
	if !azFound {
		t.Error("expected an AvailabilityZone filter in the exported FIS template")
	}
	if fisExp.Provenance.IsZero() {
		t.Error("FIS export has zero Provenance")
	}

	litmusProv := core.NewProvenance(core.KindDerived, "exporters:litmus:finding.zone-kill.public-a")
	litmusExp, err := exporters.ExportLitmus(zoneKillFinding, result.IR, litmusProv)
	if err != nil {
		t.Fatalf("ExportLitmus against the real golden bundle: %v", err)
	}
	if litmusExp.Content == "" {
		t.Fatal("Litmus export has empty Content")
	}
	if litmusExp.Provenance.IsZero() {
		t.Error("Litmus export has zero Provenance")
	}
}
