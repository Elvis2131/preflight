package core_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"preflight/core"
)

func TestSecurityGroupProfileSurvivesJSONRoundTrip(t *testing.T) {
	nodes := []core.Node{
		{ID: "lb", Type: "load_balancer"},
		{ID: "sg", Type: "network_boundary", RawAttributes: map[string]any{"security_group_rules": []map[string]any{
			{"direction": "ingress", "protocol": "tcp", "from_port": 5432, "to_port": 5432, "cidr_blocks": []string{"0.0.0.0/0"}},
		}}},
	}
	edges := []core.Edge{{From: "lb", To: "sg", Type: core.EdgeTypeDependsOn}}
	b, _ := json.Marshal(nodes)
	var rt []core.Node
	if err := json.Unmarshal(b, &rt); err != nil {
		t.Fatal(err)
	}
	direct := core.SecurityGroupProfile(nodes, edges, "lb")
	round := core.SecurityGroupProfile(rt, edges, "lb")
	if len(direct.Rules) != 1 || len(round.Rules) != 1 {
		t.Fatalf("rules direct=%d roundtrip=%d, want 1 each — the stored IR must evaluate identically to the in-memory one",
			len(direct.Rules), len(round.Rules))
	}
	if round.Rules[0].FromPort != 5432 || len(round.Rules[0].CIDRs) != 1 {
		t.Fatalf("roundtrip rule lost fields: %+v", round.Rules[0])
	}
}

// roundTrip is what the session store does to every version it saves and loads.
func roundTrip(t *testing.T, ir *core.IR) *core.IR {
	t.Helper()
	b, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	var out core.IR
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

// Every reader of an SG/NACL rule list must see a STORED (JSON-round-tripped) IR exactly
// as it sees the in-memory one. A bare []map[string]any assertion read every stored list
// as empty — the PC-137 bug, still alive in four more readers until the shared
// rawRuleMaps replaced them. Each assertion below failed before that fix.
func TestStoredIRIsReadIdenticallyToInMemoryByEveryRuleReader(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	stored := roundTrip(t, ir)

	// SG and NACL profiles
	if a, b := core.SecurityGroupProfile(ir.Nodes, ir.Edges, "db"), core.SecurityGroupProfile(stored.Nodes, stored.Edges, "db"); !reflect.DeepEqual(a, b) || len(b.Rules) == 0 {
		t.Fatalf("SG profile differs after round trip: %+v vs %+v", a, b)
	}
	if a, _ := core.NACLProfileForSubnet(ir.Nodes, ir.Edges, "subnetB"); true {
		b, ok := core.NACLProfileForSubnet(stored.Nodes, stored.Edges, "subnetB")
		if !ok || !reflect.DeepEqual(a, b) || len(b.Rules) == 0 {
			t.Fatalf("NACL profile differs after round trip: %+v vs %+v (ok=%v)", a, b, ok)
		}
	}

	// rule-change FAULTS against the stored version (the Failure Lab path)
	sgRule := core.SGRule{Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: []string{"10.0.1.0/24"}}
	if _, ok := core.WithSGRuleRemoved(stored, "sgDb", sgRule); !ok {
		t.Fatal("sg_rule_change cannot find the rule it is asked to remove in a STORED version")
	}
	naclRule := core.NACLRule{Number: 100, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: true}
	if _, ok := core.WithNACLRuleRemoved(stored, "naclB", naclRule); !ok {
		t.Fatal("nacl_rule_change cannot find the rule it is asked to remove in a STORED version")
	}

	// configuration blast surface
	j := core.DeclaredJourney{ID: "j", Name: "j", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432, Criticality: "tier1"}
	a := core.ComputeConfigurationBlastSurface(ir, j, nil)
	b := core.ComputeConfigurationBlastSurface(stored, j, nil)
	if !reflect.DeepEqual(a, b) || len(b.BreakingChanges) == 0 {
		t.Fatalf("blast surface differs after round trip:\n%+v\n%+v", a, b)
	}
}
