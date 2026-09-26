package server_test

// PC-136: GET /catalog/services must expose the real merged registry's own node
// mappings — never an invented list — so the canvas UI can offer the architect a
// real service picker per node.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	awsprovider "preflight/providers/aws"
	"preflight/server"
)

func TestListServiceCatalog_ExcludesEdgeMappings_IncludesKnownRealServices(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("awsprovider.Load(): %v", err)
	}

	entries := server.ListServiceCatalog(reg)

	byType := map[string]server.ServiceCatalogEntry{}
	for _, e := range entries {
		byType[e.ResourceType] = e
	}

	dbEntry, ok := byType["aws_db_instance"]
	if !ok {
		t.Fatal("expected aws_db_instance in the catalog — a real, mapped node type")
	}
	if dbEntry.NodeType != "managed_database" || dbEntry.CapabilityLevel != "FAILURE_SIMULATION" {
		t.Errorf("aws_db_instance = %+v, want node_type=managed_database capability_level=FAILURE_SIMULATION", dbEntry)
	}

	// aws_wafv2_web_acl_association is a real edge-only mapping (waf.yaml) — it
	// produces no IR node, so it must never appear as a selectable ServiceID.
	for _, e := range entries {
		if e.NodeType == "" {
			t.Errorf("entry %+v has an empty NodeType — an edge mapping leaked into the catalog", e)
		}
	}
}

func TestListServiceCatalog_DeterministicOrder(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("awsprovider.Load(): %v", err)
	}
	first := server.ListServiceCatalog(reg)
	second := server.ListServiceCatalog(reg)
	if len(first) != len(second) {
		t.Fatalf("length differs between calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ResourceType != second[i].ResourceType {
			t.Fatalf("order not deterministic at index %d: %q vs %q", i, first[i].ResourceType, second[i].ResourceType)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].ResourceType >= first[i].ResourceType {
			t.Errorf("not sorted at index %d: %q >= %q", i, first[i-1].ResourceType, first[i].ResourceType)
		}
	}
}

func TestListServiceCatalogHandler_ServesRealCatalogAsJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /catalog/services", server.ListServiceCatalogHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/catalog/services", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var entries []server.ServiceCatalogEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one real service catalog entry")
	}
}
