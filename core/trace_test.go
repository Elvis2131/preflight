package core_test

// PC-114: BuildTrace's own tests — a synthetic two-subnet architecture (never golden
// fixture data; PC-112/113 already established that real per-property behavior needs a
// hand-built scenario, not whatever happens to be in the golden bundles) exercising
// every pipeline step and every cutoff point named in the ticket's own reference model.

import (
	"testing"

	"preflight/core"
)

func syntheticProv() core.Provenance {
	return core.NewProvenance(core.KindDerived, "test")
}

func rt(id string, typ core.NodeType) core.Node {
	return core.Node{ID: id, Type: typ, Resolution: core.ResolutionKnown, Provenance: syntheticProv()}
}

// rtCapable is rt plus a real registry-shaped capability_level (PC-107) — every
// resource that can act as a trace source/destination in these tests needs one, the
// same key ingest/build.go's withCapabilityLevel now stamps onto every real node.
func rtCapable(id string, typ core.NodeType, level core.CapabilityLevel) core.Node {
	n := rt(id, typ)
	n.RawAttributes = map[string]any{"capability_level": string(level)}
	return n
}

// buildTwoSubnetIR builds: app (compute, subnet A) -> db (managed_database, subnet B).
// Each subnet has its own route table, NACL, and each resource its own SG. allowSG and
// allowNACL control whether the dest-side rules actually permit tcp/5432 from subnet A.
func buildTwoSubnetIR(allowSG, allowNACL bool) *core.IR {
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

	naclB := rt("naclB", core.NodeTypeNetworkBoundary)
	if allowNACL {
		naclB.RawAttributes = map[string]any{"nacl_rules": []map[string]any{
			{"number": 100, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
			{"number": 100, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
		}}
	} else {
		naclB.RawAttributes = map[string]any{"nacl_rules": []map[string]any{
			{"number": core.NACLCatchAll, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": false},
			{"number": core.NACLCatchAll, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": false},
		}}
	}
	nodes = append(nodes, naclB)

	naclA := rt("naclA", core.NodeTypeNetworkBoundary)
	naclA.RawAttributes = map[string]any{"nacl_rules": []map[string]any{
		{"number": 100, "direction": "ingress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
		{"number": 100, "direction": "egress", "protocol": "-1", "cidr": "0.0.0.0/0", "allow": true},
	}}
	nodes = append(nodes, naclA)

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

	return &core.IR{SchemaVersion: "1.1.0", VersionNumber: 1, VersionHash: "h", Nodes: nodes, Edges: edges}
}

func lastStep(tr core.Trace) core.TraceStep {
	return tr.Steps[len(tr.Steps)-1]
}

func TestBuildTrace_FullPathAllowed(t *testing.T) {
	ir := buildTwoSubnetIR(true, true)
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if !tr.Allowed {
		t.Fatalf("got denied, want allowed: %+v", tr)
	}
	wantOrder := []string{"resolve_destination", "resolve_source", "capability_check", "route_selection",
		"nacl_source_egress", "nacl_dest_ingress", "sg_dest_ingress", "target_health"}
	if len(tr.Steps) != len(wantOrder) {
		t.Fatalf("got %d steps, want %d: %+v", len(tr.Steps), len(wantOrder), tr.Steps)
	}
	for i, name := range wantOrder {
		if tr.Steps[i].Step != name {
			t.Errorf("step %d: got %q, want %q", i, tr.Steps[i].Step, name)
		}
		if tr.Steps[i].Decision != core.TraceAllow {
			t.Errorf("step %d (%s): got %s, want allow", i, name, tr.Steps[i].Decision)
		}
	}
}

func TestBuildTrace_DeniedBySG(t *testing.T) {
	ir := buildTwoSubnetIR(false, true)
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatalf("got allowed, want denied by SG: %+v", tr)
	}
	last := lastStep(tr)
	if last.Step != "sg_dest_ingress" || last.Decision != core.TraceDeny {
		t.Fatalf("got last step %+v, want a denying sg_dest_ingress step", last)
	}
}

func TestBuildTrace_DeniedByNACL(t *testing.T) {
	ir := buildTwoSubnetIR(true, false)
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatalf("got allowed, want denied by NACL: %+v", tr)
	}
	last := lastStep(tr)
	if last.Step != "nacl_dest_ingress" || last.Decision != core.TraceDeny {
		t.Fatalf("got last step %+v, want a denying nacl_dest_ingress step", last)
	}
}

func TestBuildTrace_DestinationNotFound(t *testing.T) {
	ir := buildTwoSubnetIR(true, true)
	tr := core.BuildTrace(ir, "app", "nonexistent", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatalf("got allowed, want denied: %+v", tr)
	}
	if len(tr.Steps) != 1 || tr.Steps[0].Step != "resolve_destination" {
		t.Fatalf("got steps %+v, want exactly one resolve_destination step", tr.Steps)
	}
}

func TestBuildTrace_CapabilityGate_NotAssessable(t *testing.T) {
	ir := buildTwoSubnetIR(true, true)
	// A capability_level below REQUEST_SIMULATION (e.g. a service the registry only
	// captures configuration for) must gate the pipeline, not guess.
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == "db" {
			ir.Nodes[i].RawAttributes = map[string]any{"capability_level": string(core.CapabilityConfiguration)}
		}
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatalf("got allowed, want not_assessable: %+v", tr)
	}
	last := lastStep(tr)
	if last.Step != "capability_check" || last.Decision != core.TraceNotAssessable {
		t.Fatalf("got last step %+v, want a not_assessable capability_check step", last)
	}
}

func TestBuildTrace_SameSubnet_SkipsNACL(t *testing.T) {
	ir := buildTwoSubnetIR(true, false) // NACL on subnetB denies everything
	// Put db in subnetA instead of subnetB, so app and db share a subnet.
	for i := range ir.Edges {
		if ir.Edges[i].ID == "e2" {
			ir.Edges[i].To = "subnetA"
		}
	}
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	for _, s := range tr.Steps {
		if s.Step == "nacl_source_egress" || s.Step == "nacl_dest_ingress" {
			t.Fatalf("got a NACL step for same-subnet traffic, want none: %+v", tr.Steps)
		}
	}
	if !tr.Allowed {
		t.Fatalf("got denied, want allowed (same-subnet, SG permits): %+v", tr)
	}
}

func TestBuildTrace_InternetOriginated_NACLStillApplies(t *testing.T) {
	ir := buildTwoSubnetIR(true, false) // dest NACL denies everything
	tr := core.BuildTrace(ir, "", "db", "0.0.0.0/0", "tcp", 5432)
	found := false
	for _, s := range tr.Steps {
		if s.Step == "nacl_dest_ingress" {
			found = true
			if s.Decision != core.TraceDeny {
				t.Errorf("nacl_dest_ingress: got %s, want deny", s.Decision)
			}
		}
		if s.Step == "resolve_source" || s.Step == "route_selection" {
			t.Errorf("got step %q for an internet-originated request, want none", s.Step)
		}
	}
	if !found {
		t.Fatalf("internet-originated request must still evaluate the destination subnet's NACL: %+v", tr.Steps)
	}
	if tr.Allowed {
		t.Fatalf("got allowed, want denied by dest NACL: %+v", tr)
	}
}

func TestBuildTrace_InternetOriginated_AllowedWhenPermitted(t *testing.T) {
	ir := buildTwoSubnetIR(true, true)
	// buildTwoSubnetIR's own allowSG=true fixture only opens sgDb to the internal
	// subnet range (10.0.1.0/24) — a real internet source, correctly, wouldn't match
	// that. This test is about the internet-open case, so widen sgDb's rule to
	// 0.0.0.0/0 directly.
	for i := range ir.Nodes {
		if ir.Nodes[i].ID == "sgDb" {
			ir.Nodes[i].RawAttributes = map[string]any{"security_group_rules": []map[string]any{
				{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"0.0.0.0/0"}},
			}}
		}
	}
	tr := core.BuildTrace(ir, "", "db", "0.0.0.0/0", "tcp", 5432)
	if !tr.Allowed {
		t.Fatalf("got denied, want allowed: %+v", tr)
	}
}

func TestBuildTrace_NoResolvableSubnet_NotAssessable(t *testing.T) {
	ir := buildTwoSubnetIR(true, true)
	// Sever db's contained_in edge so it has no resolvable subnet at all.
	var filtered []core.Edge
	for _, e := range ir.Edges {
		if e.ID != "e2" {
			filtered = append(filtered, e)
		}
	}
	ir.Edges = filtered
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Allowed {
		t.Fatalf("got allowed, want not_assessable: %+v", tr)
	}
	last := lastStep(tr)
	if last.Step != "route_selection" || last.Decision != core.TraceNotAssessable {
		t.Fatalf("got last step %+v, want a not_assessable route_selection step", last)
	}
}

func TestBuildTrace_ConciseSummaryMatchesSteps(t *testing.T) {
	ir := buildTwoSubnetIR(false, true)
	tr := core.BuildTrace(ir, "app", "db", "", "tcp", 5432)
	if tr.Concise == "" {
		t.Fatal("Concise summary must not be empty")
	}
	if tr.Allowed {
		t.Fatalf("fixture is set up to deny at SG: %+v", tr)
	}
}
