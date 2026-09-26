package core_test

// PC-130's own acceptance criterion: "Configuration blast surface computed for
// golden journeys, hand-verified; cap reported if hit." Golden's own checkout
// journey does not flow at baseline (see core/config_fault_test.go's own honesty
// note), so ComputeConfigurationBlastSurface correctly returns an empty result for
// it — hand-verified below, alongside a synthetic fixture proving the actual
// rule-search behavior end-to-end (real breaking rules found, cap respected).

import (
	"testing"

	"preflight/core"
)

func TestComputeConfigurationBlastSurface_Synthetic_FindsRealBreakingRule(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	j := core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}

	surface := core.ComputeConfigurationBlastSurface(ir, j, nil)
	if surface.RulesConsidered == 0 {
		t.Fatal("RulesConsidered = 0, want at least the fixture's own known SG/NACL rules")
	}
	if len(surface.BreakingChanges) == 0 {
		t.Fatal("expected at least one breaking change — sgDb's own single ingress rule is load-bearing")
	}
	found := false
	for _, e := range surface.BreakingChanges {
		if e.SGRule != nil && e.SGNodeID == "sgDb" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sgDb's own rule among BreakingChanges, got %+v", surface.BreakingChanges)
	}
}

func TestComputeConfigurationBlastSurface_RedundantRuleDoesNotBreakIt(t *testing.T) {
	// sgDb's own real ingress rule already permits tcp/5432 from 10.0.1.0/24
	// (buildFlowTestIR(true, ...)). Adding a SECOND, redundant rule granting the
	// exact same access some other way (a wider CIDR) must not itself be reported as
	// breaking — removing it alone leaves the original rule still permitting the
	// connection. This proves the search doesn't report every candidate as breaking,
	// only the ones that actually are.
	ir := buildFlowTestIR(true, true)
	node, idx := findTestNode(ir, "sgDb")
	raw, _ := node.RawAttributes["security_group_rules"].([]map[string]any)
	raw = append(raw, map[string]any{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"0.0.0.0/0"}})
	node.RawAttributes["security_group_rules"] = raw
	ir.Nodes[idx] = node

	j := core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}
	surface := core.ComputeConfigurationBlastSurface(ir, j, nil)

	// Two independently-sufficient rules now grant this connection — removing
	// EITHER one alone must leave the other still permitting it, so NEITHER should
	// be reported as a single point of failure.
	for _, e := range surface.BreakingChanges {
		if e.SGRule != nil && e.SGNodeID == "sgDb" {
			t.Errorf("sgDb rule reported as breaking, but a second, redundant rule grants the same access on its own: %+v", e)
		}
	}
}

func TestComputeConfigurationBlastSurface_BaselineDoesNotFlow_EmptyResult(t *testing.T) {
	ir := buildFlowTestIR(false, true) // SG already denies at baseline
	j := core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}

	surface := core.ComputeConfigurationBlastSurface(ir, j, nil)
	if len(surface.BreakingChanges) != 0 || surface.RulesConsidered != 0 {
		t.Fatalf("got %+v, want an empty result — a blast surface is not meaningful without a working baseline", surface)
	}
}

// TestComputeConfigurationBlastSurface_GoldenCheckout_HandVerified is the Card's own
// named golden scenario, computed exactly as it really resolves today: golden/aws's
// checkout journey does not structurally flow at baseline (PC-125's own documented
// gap), so the blast surface is correctly empty/not-applicable, not a fabricated
// non-empty result.
func TestComputeConfigurationBlastSurface_GoldenCheckout_HandVerified(t *testing.T) {
	ir := realGoldenIR(t)
	j := core.DeclaredJourney{
		ID: "checkout", Name: "checkout", Protocol: "tcp", Port: 443, Criticality: "tier1",
		Path: []string{core.JourneyInternetSentinel, "aws_lb.payments"},
	}
	baseline := core.ComputeJourneyFlow(ir, j, nil)
	if baseline.Flows {
		t.Skip("golden/aws's checkout journey now flows at baseline — PC-125's own documented NACL gap must have been closed; this test's own premise no longer holds and should be revisited")
	}
	surface := core.ComputeConfigurationBlastSurface(ir, j, nil)
	if len(surface.BreakingChanges) != 0 || surface.RulesConsidered != 0 {
		t.Fatalf("got %+v, want empty — journey does not flow at baseline", surface)
	}
}

func TestComputeConfigurationBlastSurface_CapRespected(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	// Pile on enough extra, harmless egress rules on sgApp to exceed the cap —
	// each is a distinct rule (different from_port), all permissive, none load-bearing
	// for the app->db connection actually being tested (already permitted by the
	// existing egress -1/0.0.0.0/0 rule).
	node, idx := findTestNode(ir, "sgApp")
	raw, _ := node.RawAttributes["security_group_rules"].([]map[string]any)
	for i := 0; i < core.ConfigBlastSurfaceCap+10; i++ {
		raw = append(raw, map[string]any{"direction": "egress", "protocol": "tcp", "from_port": 20000 + i, "to_port": 20000 + i, "cidr_blocks": []any{"0.0.0.0/0"}})
	}
	node.RawAttributes["security_group_rules"] = raw
	ir.Nodes[idx] = node

	j := core.DeclaredJourney{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}
	surface := core.ComputeConfigurationBlastSurface(ir, j, nil)
	if !surface.Capped {
		t.Fatalf("Capped = false, want true — %d candidate rules exceeds the cap of %d", surface.RulesConsidered, core.ConfigBlastSurfaceCap)
	}
	if surface.RulesConsidered <= core.ConfigBlastSurfaceCap {
		t.Errorf("RulesConsidered = %d, want it to report the TRUE (uncapped) count found, not the capped search size", surface.RulesConsidered)
	}
}

func findTestNode(ir *core.IR, id string) (core.Node, int) {
	for i, n := range ir.Nodes {
		if n.ID == id {
			return n, i
		}
	}
	panic("node not found: " + id)
}
