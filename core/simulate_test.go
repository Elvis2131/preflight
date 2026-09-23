package core_test

// PC-82: hand-verified against small, constructed IRs before trusting Simulate
// against the real golden architecture (core/simulate_golden_test.go covers that
// end-to-end) — the same discipline mincut.go's and delta.go's own tests established.

import (
	"encoding/json"
	"testing"

	"preflight/core"
)

func testWorkloadWithRegion(region string) core.Workload {
	return core.Workload{
		SchemaVersion:      "1.0.0",
		Name:               "test",
		Criticality:        "tier1",
		DataClassification: "PCI",
		Regions:            []string{region},
		Capacity:           map[string]float64{"app_node_rps": 100},
	}
}

func syntheticNode(id string, nodeType core.NodeType) core.Node {
	return core.Node{
		ID:         id,
		Type:       nodeType,
		Resolution: core.ResolutionKnown,
		Provenance: core.NewProvenance(core.KindStated, "test"),
	}
}

func syntheticEdge(from, to string, edgeType core.EdgeType) core.Edge {
	return core.Edge{
		ID:         from + "->" + to,
		Type:       edgeType,
		From:       from,
		To:         to,
		Resolution: core.ResolutionKnown,
		Provenance: core.NewProvenance(core.KindStated, "test"),
	}
}

func TestSimulate_RegionLoss_MatchingRegion_KillsEverything(t *testing.T) {
	ir := &core.IR{
		Nodes: []core.Node{
			syntheticNode("dns.api", core.NodeTypeDNS),
			syntheticNode("compute.app", core.NodeTypeContainerWorkload),
			syntheticNode("db.primary", core.NodeTypeManagedDatabase),
		},
		Edges: []core.Edge{
			syntheticEdge("dns.api", "compute.app", core.EdgeTypeRoutesTo),
			syntheticEdge("compute.app", "db.primary", core.EdgeTypeReadsWrites),
		},
	}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "region_loss", Target: "eu-west-1"}}, prov)

	if resp.Verdict.Value != "total_outage" {
		t.Errorf("Verdict.Value = %v, want total_outage", resp.Verdict.Value)
	}
	if len(resp.SeveredPaths) != 1 || resp.SeveredPaths[0] != "db.primary" {
		t.Errorf("SeveredPaths = %v, want [db.primary]", resp.SeveredPaths)
	}
	if len(resp.Journeys) != 0 {
		t.Errorf("Journeys = %v, want empty (no entry point survives to originate one)", resp.Journeys)
	}
	if len(resp.Cascade) != 3 {
		t.Errorf("Cascade = %v, want all 3 nodes killed", resp.Cascade)
	}
	capValue, ok := resp.Capacity.Value.(float64)
	if !ok || capValue != 0 {
		t.Errorf("Capacity.Value = %v, want 0.0 (zero surviving instances x declared rps)", resp.Capacity.Value)
	}
}

func TestSimulate_RegionLoss_NonMatchingRegion_IsNotAssessable(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{syntheticNode("dns.api", core.NodeTypeDNS)}}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "region_loss", Target: "us-east-1"}}, prov)

	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable — this workload never declared us-east-1", resp.Verdict.State)
	}
	if resp.Verdict.Reason == "" {
		t.Error("expected a non-empty reason")
	}
}

func TestSimulate_RegionLoss_MultiRegionWorkload_IsNotAssessable(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{syntheticNode("dns.api", core.NodeTypeDNS)}}
	workload := core.Workload{
		SchemaVersion: "1.0.0", Name: "test", Criticality: "tier1", DataClassification: "PCI",
		Regions: []string{"eu-west-1", "eu-west-2"},
	}
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "region_loss", Target: "eu-west-1"}}, prov)

	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable — a partial regional kill against a multi-region workload cannot be honestly computed (no per-node region attribute)", resp.Verdict.State)
	}
}

// TestSimulate_NoFaults_SeveredPathsAndJourneysAreNeverJSONNull (PC-87 fix) proves
// the wire guarantee directly, at the actual JSON boundary, not just against the Go
// slice — a nil Go slice and an empty-but-non-nil one look identical to len() but
// marshal completely differently (null vs []), and a caller that ranges over the
// decoded value (a CLI script, PC-93's eventual MCP tool, anything besides this one
// canvas that was fixed reactively) reasonably expects an array either way.
func TestSimulate_NoFaults_SeveredPathsAndJourneysAreNeverJSONNull(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{syntheticNode("dns.api", core.NodeTypeDNS)}}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, nil, prov)

	buf, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, field := range []string{"severed_paths", "journeys"} {
		if string(raw[field]) == "null" {
			t.Errorf("%s marshaled to JSON null, want [] — a nil Go slice with nothing appended must not reach the wire as null", field)
		}
	}
}

// TestSimulate_NodeLoss_KillsOnlyTheNamedNode (PC-88) is the hand-verified positive
// case: two independent paths from one entry point to two separate stateful nodes,
// one behind the killed node, one not. Only the behind-it path should sever.
func TestSimulate_NodeLoss_KillsOnlyTheNamedNode(t *testing.T) {
	ir := &core.IR{
		Nodes: []core.Node{
			syntheticNode("dns.api", core.NodeTypeDNS),
			syntheticNode("compute.a", core.NodeTypeContainerWorkload),
			syntheticNode("compute.b", core.NodeTypeContainerWorkload),
			syntheticNode("db.a", core.NodeTypeManagedDatabase),
			syntheticNode("db.b", core.NodeTypeManagedDatabase),
		},
		Edges: []core.Edge{
			syntheticEdge("dns.api", "compute.a", core.EdgeTypeRoutesTo),
			syntheticEdge("dns.api", "compute.b", core.EdgeTypeRoutesTo),
			syntheticEdge("compute.a", "db.a", core.EdgeTypeReadsWrites),
			syntheticEdge("compute.b", "db.b", core.EdgeTypeReadsWrites),
		},
	}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "node_loss", Target: "compute.a"}}, prov)

	if resp.Verdict.Value != "degraded" {
		t.Errorf("Verdict.Value = %v, want degraded (one of two paths severed)", resp.Verdict.Value)
	}
	if len(resp.SeveredPaths) != 1 || resp.SeveredPaths[0] != "db.a" {
		t.Errorf("SeveredPaths = %v, want [db.a] only — db.b's path never touches compute.a", resp.SeveredPaths)
	}
	wantCascade := []string{"compute.a", "db.a"}
	if len(resp.Cascade) != len(wantCascade) {
		t.Errorf("Cascade = %v, want %v", resp.Cascade, wantCascade)
	} else {
		for i, id := range wantCascade {
			if resp.Cascade[i] != id {
				t.Errorf("Cascade = %v, want %v", resp.Cascade, wantCascade)
			}
		}
	}
	capValue, ok := resp.Capacity.Value.(float64)
	if !ok || capValue != 100 {
		t.Errorf("Capacity.Value = %v, want 100 (one of two container_workload instances survives x 100 declared rps... see workload capacity fixture)", resp.Capacity.Value)
	}
}

// TestSimulate_NodeLoss_UnknownTarget_IsNotAssessable is the negative control:
// node_loss must never silently no-op against a target ID that isn't in the IR.
func TestSimulate_NodeLoss_UnknownTarget_IsNotAssessable(t *testing.T) {
	ir := &core.IR{Nodes: []core.Node{syntheticNode("dns.api", core.NodeTypeDNS)}}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "node_loss", Target: "compute.nonexistent"}}, prov)

	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable — target node doesn't exist in this IR", resp.Verdict.State)
	}
	if resp.Verdict.Reason == "" {
		t.Error("expected a non-empty reason")
	}
}

func TestSimulate_UnknownFaultType_IsNotAssessable(t *testing.T) {
	ir := &core.IR{}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, []core.Fault{{Type: "az_loss", Target: "eu-west-1a"}}, prov)

	if resp.Verdict.State != core.AssessmentStateNotAssessable {
		t.Errorf("Verdict.State = %q, want not_assessable — only region_loss is implemented in this version", resp.Verdict.State)
	}
}

// TestSimulate_NoFaults_Unaffected is the positive control: an empty fault list
// should never claim anything is severed.
func TestSimulate_NoFaults_Unaffected(t *testing.T) {
	ir := &core.IR{
		Nodes: []core.Node{
			syntheticNode("dns.api", core.NodeTypeDNS),
			syntheticNode("db.primary", core.NodeTypeManagedDatabase),
		},
		Edges: []core.Edge{
			syntheticEdge("dns.api", "db.primary", core.EdgeTypeRoutesTo),
		},
	}
	workload := testWorkloadWithRegion("eu-west-1")
	prov := core.NewProvenance(core.KindDerived, "test")

	resp := core.Simulate(ir, workload, nil, prov)

	if resp.Verdict.Value != "unaffected" {
		t.Errorf("Verdict.Value = %v, want unaffected", resp.Verdict.Value)
	}
	if len(resp.SeveredPaths) != 0 {
		t.Errorf("SeveredPaths = %v, want empty", resp.SeveredPaths)
	}
	if len(resp.Journeys) != 1 || !resp.Journeys[0].Survives {
		t.Errorf("Journeys = %+v, want one surviving journey", resp.Journeys)
	}
}
