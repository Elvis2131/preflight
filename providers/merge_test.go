package providers

import "testing"

// Hand-verified against small synthetic registries before trusting it against the
// real AWS+Azure data (server/azure_assess_test.go covers that end-to-end).

func TestMerge_CombinesDistinctResourceTypes(t *testing.T) {
	a := Registry{"aws_lb": {ResourceType: "aws_lb", NodeType: "load_balancer"}}
	b := Registry{"azurerm_application_gateway": {ResourceType: "azurerm_application_gateway", NodeType: "load_balancer"}}

	merged := Merge(a, b)
	if len(merged) != 2 {
		t.Fatalf("got %d entries, want 2", len(merged))
	}
	if _, ok := merged.Lookup("aws_lb"); !ok {
		t.Error("expected aws_lb to survive the merge")
	}
	if _, ok := merged.Lookup("azurerm_application_gateway"); !ok {
		t.Error("expected azurerm_application_gateway to survive the merge")
	}
}

func TestMerge_LaterRegistryWinsOnRealCollision(t *testing.T) {
	a := Registry{"widget": {ResourceType: "widget", NodeType: "compute"}}
	b := Registry{"widget": {ResourceType: "widget", NodeType: "cache"}}

	merged := Merge(a, b)
	m, ok := merged.Lookup("widget")
	if !ok {
		t.Fatal("expected widget to be present")
	}
	if m.NodeType != "cache" {
		t.Errorf("NodeType = %q, want cache (the later registry argument should win on a real key collision)", m.NodeType)
	}
}

func TestMerge_EmptyInputs_ProducesEmptyRegistry(t *testing.T) {
	merged := Merge()
	if len(merged) != 0 {
		t.Errorf("got %d entries, want 0", len(merged))
	}
}
