package core_test

// PC-131: external_dependency_outage, declared journey fallbacks, and multi-fault
// composition. Built on buildFlowTestIR's own two-subnet fixture (app -> db), plus one
// external_dependency node ("rail") standing in for a payment rail.

import (
	"reflect"
	"strings"
	"testing"

	"preflight/core"
)

func buildExternalDependencyIR() *core.IR {
	ir := buildFlowTestIR(true, true)
	ir.Nodes = append(ir.Nodes, rt("rail", core.NodeTypeExternalDependency))
	return ir
}

func railJourney(fallback *core.JourneyFallback) core.DeclaredJourney {
	return core.DeclaredJourney{
		ID: "pay", Name: "pay", Path: []string{"app", "rail"}, Protocol: "tcp", Port: 443,
		Criticality: "tier1", Fallback: fallback,
	}
}

func flowOf(t *testing.T, resp core.SimulateResponse, id string) core.JourneyFlowResult {
	t.Helper()
	for _, f := range resp.FlowDetail {
		if f.JourneyID == id {
			return f
		}
	}
	t.Fatalf("no flow_detail entry for journey %q in %+v", id, resp.FlowDetail)
	return core.JourneyFlowResult{}
}

// A journey that depends on a downed external dependency fails, and — with no declared
// fallback — is NOT degraded: graceful degradation is never assumed.
func TestSimulate_ExternalDependencyOutage_FailsDependentJourney(t *testing.T) {
	ir := buildExternalDependencyIR()
	w := core.Workload{Journeys: []core.DeclaredJourney{railJourney(nil)}}

	resp := core.Simulate(ir, w, []core.Fault{{Type: "external_dependency_outage", Target: "rail"}}, syntheticProv())
	f := flowOf(t, resp, "pay")
	if f.Flows {
		t.Fatalf("journey through a downed dependency must not flow: %+v", f)
	}
	if f.Degraded {
		t.Fatalf("no fallback was declared, so the journey must fail, never degrade: %+v", f)
	}
	if f.BlockedAt != "rail" {
		t.Errorf("BlockedAt = %q, want rail", f.BlockedAt)
	}
}

// A journey with a declared fallback that really flows under the same fault degrades
// rather than fails — and the SAME fallback stops counting the moment its own path is
// also broken (negative control: the fallback is evaluated, not merely trusted).
func TestSimulate_ExternalDependencyOutage_DeclaredFallbackDegrades(t *testing.T) {
	ir := buildExternalDependencyIR()
	fb := &core.JourneyFallback{Description: "requests buffered in a queue while the rail is down", Path: []string{"app", "db"}}
	w := core.Workload{Journeys: []core.DeclaredJourney{{
		ID: "pay", Name: "pay", Path: []string{"app", "rail"}, Protocol: "tcp", Port: 5432,
		Criticality: "tier1", Fallback: fb,
	}}}
	outage := core.Fault{Type: "external_dependency_outage", Target: "rail"}

	f := flowOf(t, core.Simulate(ir, w, []core.Fault{outage}, syntheticProv()), "pay")
	if f.Flows || !f.Degraded || f.DegradedVia != fb.Description {
		t.Fatalf("want primary not flowing + degraded via the declared fallback, got %+v", f)
	}

	// The fallback path itself is now also broken (its destination is killed too).
	f = flowOf(t, core.Simulate(ir, w, []core.Fault{outage, {Type: "node_loss", Target: "db"}}, syntheticProv()), "pay")
	if f.Flows || f.Degraded {
		t.Fatalf("a fallback whose own path is broken must not count as degraded: %+v", f)
	}
}

func TestSimulate_ExternalDependencyOutage_RefusesNonExternalOrMissingTarget(t *testing.T) {
	ir := buildExternalDependencyIR()
	for name, target := range map[string]string{"an ordinary component": "app", "a node that does not exist": "ghost"} {
		resp := core.Simulate(ir, core.Workload{}, []core.Fault{{Type: "external_dependency_outage", Target: target}}, syntheticProv())
		if resp.Verdict.State != core.AssessmentStateNotAssessable {
			t.Errorf("%s: got verdict state %q, want not_assessable (refusing to guess)", name, resp.Verdict.State)
		}
		if !strings.Contains(resp.Verdict.Reason, "external_dependency_outage") {
			t.Errorf("%s: reason %q should name the fault type", name, resp.Verdict.Reason)
		}
	}
}

// buildTwoDatabaseIR is buildFlowTestIR plus a second database ("db2") in the same
// subnet with its own SG allowing the same port, so two journeys from "app" both flow
// at baseline and each depends on a DIFFERENT thing (sgDb vs db2 itself).
func buildTwoDatabaseIR() *core.IR {
	ir := buildFlowTestIR(true, true)
	prov := syntheticProv()
	sgDb2 := rt("sgDb2", core.NodeTypeNetworkBoundary)
	sgDb2.RawAttributes = map[string]any{"security_group_rules": []map[string]any{
		{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []any{"10.0.1.0/24"}},
	}}
	ir.Nodes = append(ir.Nodes, rtCapable("db2", core.NodeTypeManagedDatabase, core.CapabilityFailureSimulation), sgDb2)
	ir.Edges = append(ir.Edges,
		core.Edge{ID: "e11", Type: core.EdgeTypeContainedIn, From: "db2", To: "subnetB", Resolution: core.ResolutionKnown, Provenance: prov},
		core.Edge{ID: "e12", Type: core.EdgeTypeDependsOn, From: "db2", To: "sgDb2", Resolution: core.ResolutionKnown, Provenance: prov},
	)
	return ir
}

// Composition (PC-131's own named hiding place for subtle bugs): applying two faults
// together must equal applying BOTH to the model, not the union of two separate
// results. One fault is an IR mutation (sg_rule_change on sgDb), the other a kill
// (node_loss of db2); each breaks a different journey, and the composed result must
// equal Simulate over an IR that already has the SG mutation baked in.
func TestSimulate_MultiFault_ComposesAsBothMutationsApplied(t *testing.T) {
	ir := buildTwoDatabaseIR()
	w := core.Workload{Journeys: []core.DeclaredJourney{
		{ID: "j1", Name: "j1", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"},
		{ID: "j2", Name: "j2", Path: []string{"app", "db2"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"},
	}}
	sgFault := core.Fault{Type: "sg_rule_change", Target: "sgDb", SGRuleRemove: &core.SGRule{
		Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"10.0.1.0/24"},
	}}
	killFault := core.Fault{Type: "node_loss", Target: "db2"}

	base := core.Simulate(ir, w, nil, syntheticProv())
	if !flowOf(t, base, "j1").Flows || !flowOf(t, base, "j2").Flows {
		t.Fatalf("test setup: both journeys must flow at baseline: %+v", base.FlowDetail)
	}

	// Each fault alone breaks exactly its own journey, so a composition that only
	// honours the first (or last) fault cannot pass.
	onlySG := core.Simulate(ir, w, []core.Fault{sgFault}, syntheticProv())
	if flowOf(t, onlySG, "j1").Flows || !flowOf(t, onlySG, "j2").Flows {
		t.Fatalf("sg fault alone should break only j1: %+v", onlySG.FlowDetail)
	}
	onlyKill := core.Simulate(ir, w, []core.Fault{killFault}, syntheticProv())
	if !flowOf(t, onlyKill, "j1").Flows || flowOf(t, onlyKill, "j2").Flows {
		t.Fatalf("kill fault alone should break only j2: %+v", onlyKill.FlowDetail)
	}

	composed := core.Simulate(ir, w, []core.Fault{sgFault, killFault}, syntheticProv())
	if flowOf(t, composed, "j1").Flows || flowOf(t, composed, "j2").Flows {
		t.Fatalf("both journeys must be broken under the composed fault: %+v", composed.FlowDetail)
	}

	mutated, ok := core.WithSGRuleRemoved(ir, "sgDb", *sgFault.SGRuleRemove)
	if !ok {
		t.Fatal("test setup: sg rule to remove not found")
	}
	bothApplied := core.Simulate(mutated, w, []core.Fault{killFault}, syntheticProv())
	if !reflect.DeepEqual(composed.FlowDetail, bothApplied.FlowDetail) {
		t.Fatalf("composed faults != both applied to the model\ncomposed:     %+v\nboth-applied: %+v", composed.FlowDetail, bothApplied.FlowDetail)
	}

	// Order must not matter either: faults are a set applied to one model.
	reversed := core.Simulate(ir, w, []core.Fault{killFault, sgFault}, syntheticProv())
	if !reflect.DeepEqual(composed.FlowDetail, reversed.FlowDetail) {
		t.Fatalf("fault order changed the result:\n%+v\nvs\n%+v", composed.FlowDetail, reversed.FlowDetail)
	}
}
