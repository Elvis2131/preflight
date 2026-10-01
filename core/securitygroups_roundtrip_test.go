package core_test

import (
	"encoding/json"
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
