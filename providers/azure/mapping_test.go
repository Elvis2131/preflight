package azure

import "testing"

var golden8 = []string{
	"azurerm_dns_a_record",
	"azurerm_web_application_firewall_policy",
	"azurerm_application_gateway",
	"azurerm_kubernetes_cluster",
	"azurerm_mssql_database",
	"azurerm_redis_cache",
	"azurerm_servicebus_queue",
	"azurerm_role_assignment",
}

// TestAll8GoldenResourceTypesHaveMappings is PC-22's first acceptance criterion:
// "All 8 Azure resource types (Azure DNS, Front Door, App Gateway, AKS, Azure SQL,
// Azure Cache for Redis, Service Bus, Entra/RBAC) are mapped." Front Door itself is
// out of scope (see application_gateway.yaml's own doc comment for why, and the
// explicit decision that produced it) — Application Gateway's own attached WAF policy
// fills the network_boundary slot Front Door would otherwise have.
func TestAll8GoldenResourceTypesHaveMappings(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if len(reg) != len(golden8) {
		t.Errorf("Load() returned %d mappings, want exactly %d — got: %v", len(reg), len(golden8), reg)
	}
	for _, rt := range golden8 {
		m, ok := reg.Lookup(rt)
		if !ok {
			t.Errorf("no mapping loaded for golden resource type %q", rt)
			continue
		}
		if m.IsEdgeMapping() {
			t.Errorf("golden resource type %q is mapped as an edge, but all 8 golden types must produce a node", rt)
		}
	}
}

// PC-22's third acceptance criterion — "No changes to core/analyse were required to
// support Azure" — is proven structurally the same way providers/aws/mapping_test.go
// already proves it for AWS (core/internal/analyse imports no providers package at
// all, checked by parsing its own source). The REAL end-to-end proof that ingest
// itself needs no azure-specific branch — a synthetic Azure bundle actually producing
// a real IR node through the unmodified ingest.Ingest — lives in
// ingest/azure_test.go, not here: it needs to import preflight/ingest, which this
// package's own tests should not depend on (providers/azure is a leaf package ingest
// itself depends ON, not the reverse).
