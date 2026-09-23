package exporters_test

// PC-29: "region-loss simulation verified by an exported chaos experiment", hand-
// verified before trusting it against the real golden workload.

import (
	"encoding/json"
	"testing"

	"preflight/core"
	"preflight/exporters"
	"preflight/ingest"
)

func TestExportFISRegionLoss_ProducesValidJSONScopedToTheWorkload(t *testing.T) {
	workload := core.Workload{
		SchemaVersion: "1.0.0", Name: "test-workload", Criticality: "tier1", DataClassification: "PCI",
		Regions: []string{"eu-west-1"},
	}
	prov := core.NewProvenance(core.KindDerived, "test:export-fis-region-loss")

	exp, err := exporters.ExportFISRegionLoss(workload, prov)
	if err != nil {
		t.Fatalf("ExportFISRegionLoss: %v", err)
	}
	if exp.Format != "fis" {
		t.Errorf("Format = %q, want fis", exp.Format)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(exp.Content), &parsed); err != nil {
		t.Fatalf("Content is not valid JSON: %v", err)
	}
	targets := parsed["targets"].(map[string]any)
	target := targets["AllWorkloadInstances"].(map[string]any)
	tags := target["resourceTags"].(map[string]any)
	if tags["Service"] != "test-workload" {
		t.Errorf("resourceTags.Service = %v, want test-workload (the workload's own Name)", tags["Service"])
	}
	if _, hasAZFilter := target["filters"]; hasAZFilter {
		t.Error("region-loss export must have NO availability-zone filter — that would scope it back to a single zone")
	}
}

func TestExportFISRegionLoss_EmptyWorkloadName_ReturnsError(t *testing.T) {
	workload := core.Workload{SchemaVersion: "1.0.0", Criticality: "tier1", DataClassification: "PCI", Regions: []string{"eu-west-1"}}
	prov := core.NewProvenance(core.KindDerived, "test")

	_, err := exporters.ExportFISRegionLoss(workload, prov)
	if err == nil {
		t.Fatal("expected an error for an unnamed workload — refuse an unscoped, dangerously broad target")
	}
}

func TestExportFISRegionLoss_AgainstGoldenWorkload(t *testing.T) {
	workload, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	if workload.Name != "payments-api" {
		t.Fatalf("precondition failed: golden/workload.yaml name=%q, want payments-api", workload.Name)
	}

	prov := core.NewProvenance(core.KindDerived, "test:export-fis-region-loss")
	exp, err := exporters.ExportFISRegionLoss(workload, prov)
	if err != nil {
		t.Fatalf("ExportFISRegionLoss against the real golden workload: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(exp.Content), &parsed); err != nil {
		t.Fatalf("Content is not valid JSON: %v", err)
	}
	targets := parsed["targets"].(map[string]any)
	target := targets["AllWorkloadInstances"].(map[string]any)
	tags := target["resourceTags"].(map[string]any)
	// Verified against golden/aws/network.tf directly: the VPC (and other golden
	// resources) carry the literal tag Service = "payments-api" — this is a real,
	// checked correlation, not an assumed match.
	if tags["Service"] != "payments-api" {
		t.Errorf("resourceTags.Service = %v, want payments-api (matches golden/aws/network.tf's own real Service tag)", tags["Service"])
	}
}
