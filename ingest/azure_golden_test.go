package ingest_test

// PC-22: hand-verification of the real golden/azure bundle, locked in as a permanent
// test — the same "derive then hand-verify" discipline PC-15 established for
// golden/aws (don't trust generated fixture output by construction; confirm it
// against the real Terraform directly, then pin it here so a future regression is
// caught).

import (
	"testing"

	"preflight/ingest"
	azureprovider "preflight/providers/azure"
)

func TestIngest_GoldenAzureBundle_HandVerified(t *testing.T) {
	reg, err := azureprovider.Load()
	if err != nil {
		t.Fatalf("providers/azure.Load(): %v", err)
	}

	dir := findGoldenAzureDir(t)
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest(%s): %v", dir, err)
	}
	if result.Insufficient != nil {
		t.Fatalf("golden/azure unexpectedly failed the MVG check: %+v", result.Insufficient)
	}

	// Exactly the golden 8, confirmed against golden/azure/*.tf directly: App Gateway,
	// its WAF policy, AKS, Azure SQL, Redis, Service Bus queue, DNS record, RBAC role
	// assignment. Network substrate (resource group, VNet, subnets, NSG, public IP,
	// DNS zone, SQL server, Service Bus namespace) is deliberately unmapped — the same
	// PC-13-vs-PC-78 scope split AWS itself has, not an oversight (see
	// golden/azure/README.md).
	if len(result.IR.Nodes) != 8 {
		t.Errorf("got %d nodes, want 8 (the golden 8, network substrate stays out-of-vocabulary): %+v", len(result.IR.Nodes), nodeIDs(result))
	}

	// Exactly 2 edges, confirmed by direct inspection of the generated IR, not
	// assumed: Application Gateway -> its WAF policy (firewall_policy_id), and the
	// RBAC role assignment -> AKS (principal_id referencing AKS's own managed
	// identity). dns -> load_balancer does NOT form — see dns.tf's own doc comment for
	// why (target_resource_id routes through an unmapped Public IP resource, a real
	// Azure/AWS topological difference, not a bug).
	if len(result.IR.Edges) != 2 {
		t.Errorf("got %d edges, want 2: %+v", len(result.IR.Edges), result.IR.Edges)
	}

	sql := findAzureNode(t, result, "azurerm_mssql_database.payments")
	if sql.Capability == nil {
		t.Fatal("expected azurerm_mssql_database.payments to have a populated Capability")
	}
	if sql.Capability.ReplicationMode == nil || *sql.Capability.ReplicationMode != "sync" {
		t.Errorf("SQL ReplicationMode = %v, want sync (zone_redundant=true)", sql.Capability.ReplicationMode)
	}
	if sql.Capability.EncryptionMechanism == nil || *sql.Capability.EncryptionMechanism != "true" {
		t.Errorf("SQL EncryptionMechanism = %v, want \"true\" (transparent_data_encryption_enabled=true)", sql.Capability.EncryptionMechanism)
	}

	appgw := findAzureNode(t, result, "azurerm_application_gateway.payments")
	wafEdgeFound := false
	for _, e := range result.IR.Edges {
		if e.From == appgw.ID && e.To == "azurerm_web_application_firewall_policy.payments" {
			wafEdgeFound = true
		}
	}
	if !wafEdgeFound {
		t.Error("expected an edge from Application Gateway to its WAF policy (firewall_policy_id reference)")
	}
}

func nodeIDs(result ingest.Result) []string {
	ids := make([]string, len(result.IR.Nodes))
	for i, n := range result.IR.Nodes {
		ids[i] = n.ID
	}
	return ids
}
