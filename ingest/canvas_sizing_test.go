package ingest_test

// PC-110: canvas-entered sizing round-trips into core.Sizing, and end to end into a
// real cost result — blank sizing must stay cost_unknown, never a guessed default.

import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

func TestIngestCanvas_Sizing_PopulatedFieldsTranslateToTypedSizing(t *testing.T) {
	doc := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, Sizing: map[string]string{
				"instance_class":       "db.r6g.xlarge",
				"allocated_storage_gb": "100",
				"storage_type":         "gp3",
			}},
		},
		Edges: []core.CanvasEdge{{ID: "e1", Type: "routes_to", From: "lb", To: "db"}},
	}
	result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	if result.Insufficient != nil {
		t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
	}
	var db core.Node
	for _, n := range result.IR.Nodes {
		if n.ID == "db" {
			db = n
		}
	}
	if db.Sizing == nil {
		t.Fatal("Sizing is nil, want a populated core.Sizing")
	}
	if db.Sizing.InstanceClass == nil || *db.Sizing.InstanceClass != "db.r6g.xlarge" {
		t.Errorf("InstanceClass = %v, want db.r6g.xlarge", db.Sizing.InstanceClass)
	}
	if db.Sizing.AllocatedStorageGB == nil || *db.Sizing.AllocatedStorageGB != 100 {
		t.Errorf("AllocatedStorageGB = %v, want 100", db.Sizing.AllocatedStorageGB)
	}
	if db.Sizing.StorageType == nil || *db.Sizing.StorageType != "gp3" {
		t.Errorf("StorageType = %v, want gp3", db.Sizing.StorageType)
	}
}

func TestIngestCanvas_Sizing_AbsentOrBlank_NilSizing(t *testing.T) {
	cases := []struct {
		name   string
		sizing map[string]string
	}{
		{"nil map", nil},
		{"empty map", map[string]string{}},
		{"blank values", map[string]string{"instance_class": "", "allocated_storage_gb": ""}},
		{"unparsable int", map[string]string{"allocated_storage_gb": "not-a-number"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := core.CanvasDocument{
				Nodes: []core.CanvasNode{
					{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
					{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{}, Sizing: tc.sizing},
				},
				Edges: []core.CanvasEdge{{ID: "e1", Type: "routes_to", From: "lb", To: "db"}},
			}
			result, err := ingest.IngestCanvas(doc, providers.Registry{}, 1)
			if err != nil {
				t.Fatalf("IngestCanvas: %v", err)
			}
			if result.Insufficient != nil {
				t.Fatalf("unexpectedly insufficient: %+v", result.Insufficient)
			}
			var db core.Node
			for _, n := range result.IR.Nodes {
				if n.ID == "db" {
					db = n
				}
			}
			if db.Sizing != nil {
				t.Errorf("Sizing = %+v, want nil — cost_unknown must never be a guessed default", db.Sizing)
			}
		})
	}
}

// TestIngestCanvas_Sizing_EndToEndCost is PC-110's own acceptance criterion,
// verbatim: "Blank sizing produces cost_unknown/not_assessable in the cost result,
// verified end to end, not defaulted." Reuses core.ComputeCost unchanged (PC-117) —
// this test proves the canvas's own new sizing input reaches that real engine
// correctly, not a new cost computation invented for canvas.
func TestIngestCanvas_Sizing_EndToEndCost(t *testing.T) {
	priced := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{
				// "engine" is not a CapabilityModel field, but canvasCapabilityToRawAttributes
				// copies every capability key into RawAttributes verbatim regardless — the
				// same real key matchRDSRow (core/cost.go) reads, mirroring a real
				// Terraform aws_db_instance's own "engine" attribute.
				ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{"engine": "postgres"},
				Sizing: map[string]string{"instance_class": "db.r6g.xlarge"},
			},
		},
		Edges: []core.CanvasEdge{{ID: "e1", Type: "routes_to", From: "lb", To: "db"}},
	}
	table := core.PriceTable{SnapshotID: "test", Rows: []core.PriceRow{
		{Service: "AmazonRDS", Unit: "Hrs", Price: 0.899, Currency: "USD",
			SKUAttributes: map[string]string{"instanceType": "db.r6g.xlarge", "databaseEngine": "PostgreSQL", "deploymentOption": "Single-AZ"}},
	}}

	prov := core.NewProvenance(core.KindDerived, "test")

	pricedResult, err := ingest.IngestCanvas(priced, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	pricedReport := core.ComputeCost(pricedResult.IR, table, prov)
	if componentFor(pricedReport, "db").Decision != core.CostPriced {
		t.Errorf("with sizing: Decision = %q, want priced: %s", componentFor(pricedReport, "db").Decision, componentFor(pricedReport, "db").Reason)
	}

	blank := core.CanvasDocument{
		Nodes: []core.CanvasNode{
			{ID: "lb", Type: "load_balancer", Label: "LB", Capability: map[string]string{}},
			{ID: "db", Type: "managed_database", Label: "DB", Capability: map[string]string{"engine": "postgres"}},
		},
		Edges: []core.CanvasEdge{{ID: "e1", Type: "routes_to", From: "lb", To: "db"}},
	}
	blankResult, err := ingest.IngestCanvas(blank, providers.Registry{}, 1)
	if err != nil {
		t.Fatalf("IngestCanvas: %v", err)
	}
	blankReport := core.ComputeCost(blankResult.IR, table, prov)
	if componentFor(blankReport, "db").Decision != core.CostUnknown {
		t.Errorf("blank sizing: Decision = %q, want cost_unknown", componentFor(blankReport, "db").Decision)
	}
}

func componentFor(report core.CostReport, nodeID string) core.ComponentCost {
	for _, c := range report.Components {
		if c.NodeID == nodeID {
			return c
		}
	}
	panic("component not found: " + nodeID)
}
