package core_test

// PC-125: ComputeJourneyFlow's own tests against a hand-built synthetic architecture
// (the same discipline PC-112/113/114 established: real per-property behavior needs
// a scenario built to exercise it, not whatever the golden bundle happens to contain).

import (
	"reflect"
	"strings"
	"testing"

	"preflight/core"
)

// buildFlowTestIR mirrors core/trace_test.go's own two-subnet fixture: app (compute,
// subnet A) -> db (managed_database, subnet B), each with its own SG/NACL/route
// table. allowSG/allowNACL control whether the dest-side rules actually permit the
// connection — reused here to prove journey flow correctly cites a blocked hop.
func buildFlowTestIR(allowSG, allowNACL bool) *core.IR {
	prov := syntheticProv()
	subnetA := rt("subnetA", core.NodeTypeNetworkBoundary)
	subnetA.RawAttributes = map[string]any{"cidr_block": "10.0.1.0/24"}
	subnetB := rt("subnetB", core.NodeTypeNetworkBoundary)
	subnetB.RawAttributes = map[string]any{"cidr_block": "10.0.2.0/24"}

	nodes := []core.Node{
		rtCapable("app", core.NodeTypeCompute, core.CapabilityRequestSimulation),
		rtCapable("db", core.NodeTypeManagedDatabase, core.CapabilityFailureSimulation),
		subnetA,
		subnetB,
		rt("rtA", core.NodeTypeNetworkBoundary),
		rt("rtB", core.NodeTypeNetworkBoundary),
		rt("target", core.NodeTypeNetworkBoundary),
	}

	sgApp := rt("sgApp", core.NodeTypeNetworkBoundary)
	sgApp.RawAttributes = map[string]any{"security_group_rules": []map[string]any{
		{"direction": "egress", "protocol": "-1", "cidr_blocks": []any{"0.0.0.0/0"}},
	}}
	nodes = append(nodes, sgApp)

	sgDb := rt("sgDb", core.NodeTypeNetworkBoundary)
	if allowSG {
		sgDb.RawAttributes = map[string]any{"security_group_rules": []map[string]any{
			{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"10.0.1.0/24"}},
		}}
	} else {
		sgDb.RawAttributes = map[string]any{"security_group_rules": []map[string]any{
			{"direction": "ingress", "protocol": "tcp", "from_port": 22, "to_port": 22, "cidr_blocks": []any{"10.0.1.0/24"}},
		}}
	}
	nodes = append(nodes, sgDb)

	naclRules := func(allow bool) map[string]any {
		if allow {
			return map[string]any{"nacl_rules": []map[string]any{
				{"number": 100, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
				{"number": 100, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
			}}
		}
		return map[string]any{"nacl_rules": []map[string]any{
			{"number": core.NACLCatchAll, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": false},
			{"number": core.NACLCatchAll, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": false},
		}}
	}
	naclA := rt("naclA", core.NodeTypeNetworkBoundary)
	naclA.RawAttributes = naclRules(true)
	naclB := rt("naclB", core.NodeTypeNetworkBoundary)
	naclB.RawAttributes = naclRules(allowNACL)
	nodes = append(nodes, naclA, naclB)

	edges := []core.Edge{
		{ID: "e1", Type: core.EdgeTypeContainedIn, From: "app", To: "subnetA", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e2", Type: core.EdgeTypeContainedIn, From: "db", To: "subnetB", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e3", Type: core.EdgeTypeDependsOn, From: "subnetA", To: "rtA", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e4", Type: core.EdgeTypeDependsOn, From: "subnetB", To: "rtB", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e5", Type: core.EdgeTypeRoutesTo, From: "rtA", To: "target", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e6", Type: core.EdgeTypeRoutesTo, From: "rtB", To: "target", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e7", Type: core.EdgeTypeDependsOn, From: "app", To: "sgApp", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e8", Type: core.EdgeTypeDependsOn, From: "db", To: "sgDb", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e9", Type: core.EdgeTypeDependsOn, From: "subnetA", To: "naclA", Resolution: core.ResolutionKnown, Provenance: prov},
		{ID: "e10", Type: core.EdgeTypeDependsOn, From: "subnetB", To: "naclB", Resolution: core.ResolutionKnown, Provenance: prov},
	}
	return &core.IR{SchemaVersion: "1.2.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: edges}
}

func TestComputeJourneyFlow_FullyFlows(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	if !flow.Flows {
		t.Fatalf("got Flows=false, want true: %+v", flow)
	}
	if len(flow.Hops) != 1 || !flow.Hops[0].Allowed {
		t.Fatalf("got %+v, want one allowed hop", flow.Hops)
	}
}

// TestComputeJourneyFlow_BlockedBySGNACL is PC-125's own acceptance criterion,
// verbatim: "A journey blocked by an SG/NACL/route decision is reported as not
// flowing, citing the trace step."
func TestComputeJourneyFlow_BlockedBySG_CitesTraceStep(t *testing.T) {
	ir := buildFlowTestIR(false, true) // SG denies, NACL allows
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	if flow.Flows {
		t.Fatal("got Flows=true, want false (SG denies this port)")
	}
	if !strings.Contains(flow.BlockedReason, "sg_dest_ingress") {
		t.Errorf("BlockedReason = %q, want it to cite the sg_dest_ingress trace step", flow.BlockedReason)
	}
}

func TestComputeJourneyFlow_BlockedByNACL_CitesTraceStep(t *testing.T) {
	ir := buildFlowTestIR(true, false) // SG allows, NACL denies
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	if flow.Flows {
		t.Fatal("got Flows=true, want false (dest NACL denies)")
	}
	if !strings.Contains(flow.BlockedReason, "nacl_dest_ingress") {
		t.Errorf("BlockedReason = %q, want it to cite the nacl_dest_ingress trace step", flow.BlockedReason)
	}
}

// TestComputeJourneyFlow_StopsUnderFault is PC-125's own acceptance criterion:
// "Under AZ loss, journeys ... stop where they don't [have a surviving path]."
func TestComputeJourneyFlow_StopsUnderFault(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	killed := map[string]bool{"db": true} // e.g. db's own AZ/subnet was killed
	flow := core.ComputeJourneyFlow(ir, j, killed)
	if flow.Flows {
		t.Fatal("got Flows=true, want false (db is in the killed set)")
	}
	if flow.BlockedAt != "db" {
		t.Errorf("BlockedAt = %q, want db", flow.BlockedAt)
	}
	if len(flow.Hops) != 1 || flow.Hops[0].Allowed {
		t.Fatalf("got %+v, want one denied hop, and BuildTrace must never even be consulted for a killed component", flow.Hops)
	}
}

func TestComputeJourneyFlow_InternetSentinel(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Path: []string{core.JourneyInternetSentinel, "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	// db's SG only allows 10.0.1.0/24 (subnetA's own range) — an internet source
	// (0.0.0.0/0) does not match a CIDR-restricted rule, so this must NOT silently
	// flow; the real, honest result here is a deny/not_assessable, never a guess.
	if flow.Flows {
		t.Fatalf("got Flows=true — an internet source should not match a rule restricted to 10.0.1.0/24, got %+v", flow)
	}
}

// TestComputeJourneyFlow_RealAZLossViaContainmentBlastRadius proves the Card's own
// "reuses PC-14's containment-closure and reachability logic; do not duplicate it"
// instruction directly: killing subnetB via core.ContainmentBlastRadius (PC-14's own
// engine, not reimplemented here) correctly stops the journey at db, since db is
// really contained_in subnetB.
func TestComputeJourneyFlow_RealAZLossViaContainmentBlastRadius(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	var containmentEdges []core.DirectedEdge
	for _, e := range ir.Edges {
		if e.Type == core.EdgeTypeContainedIn {
			containmentEdges = append(containmentEdges, core.DirectedEdge{From: e.From, To: e.To})
		}
	}
	blastRadius := core.ContainmentBlastRadius(containmentEdges, "subnetB")
	killed := map[string]bool{"subnetB": true}
	for _, id := range blastRadius {
		killed[id] = true
	}
	if !killed["db"] {
		t.Fatal("expected db to be in subnetB's own containment blast radius")
	}

	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, killed)
	if flow.Flows {
		t.Fatal("got Flows=true, want false — db's own subnet was killed")
	}
	if flow.BlockedAt != "db" {
		t.Errorf("BlockedAt = %q, want db", flow.BlockedAt)
	}
}

func TestComputeAllJourneyFlows_DeterministicOrder(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	workload := core.Workload{Journeys: []core.DeclaredJourney{
		{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432},
		{ID: "j2", Path: []string{"app", "db"}, Protocol: "tcp", Port: 22},
	}}
	r1 := core.ComputeAllJourneyFlows(ir, workload, nil)
	r2 := core.ComputeAllJourneyFlows(ir, workload, nil)
	if len(r1) != 2 || r1[0].JourneyID != "j1" || r1[1].JourneyID != "j2" {
		t.Fatalf("got %+v, want [j1, j2] in declared order", r1)
	}
	for i := range r1 {
		if !reflect.DeepEqual(r1[i], r2[i]) {
			t.Fatalf("non-deterministic: run 1 %+v != run 2 %+v", r1[i], r2[i])
		}
	}
}
