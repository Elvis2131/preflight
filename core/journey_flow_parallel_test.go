package core_test

// PC-125/126's own follow-up (filed after PC-136 surfaced that the "parallel paths"
// scope gap blocks the Card's own named example — "2 of 3 app nodes lost, survivors
// at 150% of declared capacity" — from ever being built or tested): a Path element
// joined by "|" declares parallel members at that position. These tests exercise the
// real engine change (core/journey_flow.go), not a synthetic stand-in.

import (
	"testing"

	"preflight/core"
)

// addParallelAppReplicas extends buildFlowTestIR's own "app" fixture with two more
// identically-placed, identically-secured replica nodes (app2, app3) — same subnet,
// same security group — so all three are real, structurally independent alternatives
// to reach "db", not a single node pretending to be three.
func addParallelAppReplicas(ir *core.IR) *core.IR {
	prov := syntheticProv()
	for _, id := range []string{"app2", "app3"} {
		ir.Nodes = append(ir.Nodes, rtCapable(id, core.NodeTypeCompute, core.CapabilityRequestSimulation))
		ir.Edges = append(ir.Edges,
			core.Edge{ID: id + "-subnet", Type: core.EdgeTypeContainedIn, From: id, To: "subnetA", Resolution: core.ResolutionKnown, Provenance: prov},
			core.Edge{ID: id + "-sg", Type: core.EdgeTypeDependsOn, From: id, To: "sgApp", Resolution: core.ResolutionKnown, Provenance: prov},
		)
	}
	return ir
}

func TestComputeJourneyFlow_ParallelGroup_AllHealthy_FlowsWithAllThreeReached(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)

	if !flow.Flows {
		t.Fatalf("got Flows=false, want true: %+v", flow)
	}
	if len(flow.ReachedByGroup) != 2 {
		t.Fatalf("got %d ReachedByGroup entries, want 2 (start group, db group)", len(flow.ReachedByGroup))
	}
	if got := flow.ReachedByGroup[0]; len(got) != 3 {
		t.Errorf("ReachedByGroup[0] = %v, want all 3 replicas reached", got)
	}
	if got := flow.ReachedByGroup[1]; len(got) != 1 || got[0] != "db" {
		t.Errorf("ReachedByGroup[1] = %v, want [db]", got)
	}
	// Every replica -> db pair is independently evaluated (cross-product), not
	// short-circuited after the first success — real evidence, not an assumption.
	if len(flow.Hops) != 3 {
		t.Fatalf("got %d hops, want 3 (one per replica -> db pair)", len(flow.Hops))
	}
	for _, h := range flow.Hops {
		if !h.Allowed {
			t.Errorf("hop %+v: want Allowed=true — all three replicas share the same subnet/SG as the original single-app fixture", h)
		}
		if h.GroupIndex != 1 {
			t.Errorf("hop %+v: GroupIndex = %d, want 1 (db's own position in Path)", h, h.GroupIndex)
		}
	}
}

func TestComputeJourneyFlow_ParallelGroup_OneReplicaKilled_ReroutesToSurvivors(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432}

	flow := core.ComputeJourneyFlow(ir, j, map[string]bool{"app": true})
	if !flow.Flows {
		t.Fatalf("got Flows=false, want true — 2 of 3 replicas survive: %+v", flow)
	}
	if got := flow.ReachedByGroup[0]; len(got) != 2 || got[0] != "app2" || got[1] != "app3" {
		t.Errorf("ReachedByGroup[0] = %v, want [app2 app3] — the real reroute-to-surviving-members set", got)
	}
}

func TestComputeJourneyFlow_ParallelGroup_AllReplicasKilled_Stops(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app|app2|app3", "db"}, Protocol: "tcp", Port: 5432}

	flow := core.ComputeJourneyFlow(ir, j, map[string]bool{"app": true, "app2": true, "app3": true})
	if flow.Flows {
		t.Fatal("got Flows=true, want false — every parallel member is killed")
	}
	if flow.BlockedAt != "app" { // alphabetically first among the killed group
		t.Errorf("BlockedAt = %q, want %q (deterministic: alphabetically-first group member)", flow.BlockedAt, "app")
	}
}

func TestComputeJourneyFlow_ParallelGroup_DeterministicAcrossRuns(t *testing.T) {
	ir := addParallelAppReplicas(buildFlowTestIR(true, true))
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app3|app|app2", "db"}, Protocol: "tcp", Port: 5432} // declared out of alphabetical order
	first := core.ComputeJourneyFlow(ir, j, nil)
	second := core.ComputeJourneyFlow(ir, j, nil)
	if len(first.Hops) != len(second.Hops) {
		t.Fatalf("hop count differs between runs: %d vs %d", len(first.Hops), len(second.Hops))
	}
	for i := range first.Hops {
		if first.Hops[i] != second.Hops[i] {
			t.Fatalf("hop %d differs between runs: %+v vs %+v (NFR-1 violation)", i, first.Hops[i], second.Hops[i])
		}
	}
}

// TestComputeJourneyFlow_SingletonPath_Unaffected is a direct regression guard for
// this file's own additive-only claim: a Path with no "|" anywhere must produce
// EXACTLY the same Hops/Flows/BlockedAt/BlockedReason this package's pre-existing
// tests already lock in — checked here again, explicitly, alongside the new
// parallel-group behaviour rather than trusting the old test file alone.
func TestComputeJourneyFlow_SingletonPath_Unaffected(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432}
	flow := core.ComputeJourneyFlow(ir, j, nil)
	if !flow.Flows || len(flow.Hops) != 1 || flow.Hops[0].From != "app" || flow.Hops[0].To != "db" || !flow.Hops[0].Allowed {
		t.Fatalf("got %+v, want a single allowed app->db hop", flow)
	}
	if len(flow.ReachedByGroup) != 2 || len(flow.ReachedByGroup[0]) != 1 || len(flow.ReachedByGroup[1]) != 1 {
		t.Errorf("ReachedByGroup = %v, want two singleton groups", flow.ReachedByGroup)
	}
}
