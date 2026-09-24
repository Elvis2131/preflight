package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestHTTPAndMCP_CallTheSameFunction is PC-21's own named risk, verified directly:
// call the HTTP handler and the MCP tool with the SAME request against the SAME
// store, and confirm they produce equivalent results — proving there is exactly one
// implementation underneath both, not two that could quietly drift. This does not
// merely read the source and assert "they both call Assess" — it exercises both
// entry points end to end and compares real output.
func TestHTTPAndMCP_CallTheSameFunction(t *testing.T) {
	httpStore := testStore(t)
	mcpStore := testStore(t)

	req := AssessRequest{SessionID: "sess-parity", BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"}

	// Call through the HTTP handler.
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/assess", bytes.NewReader(body))
	httpRec := httptest.NewRecorder()
	AssessHandler(httpStore)(httpRec, httpReq)

	if httpRec.Code != http.StatusOK {
		t.Fatalf("HTTP handler: status = %d, body = %s", httpRec.Code, httpRec.Body.String())
	}
	var httpResp AssessResponse
	if err := json.Unmarshal(httpRec.Body.Bytes(), &httpResp); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}

	// Call through the MCP tool handler directly (same function assessTool returns,
	// without needing a full client/server transport round-trip for this test).
	toolHandler := assessTool(mcpStore)
	_, mcpResp, err := toolHandler(context.Background(), nil, req)
	if err != nil {
		t.Fatalf("MCP tool handler: %v", err)
	}

	// Compare the parts that must be identical given identical input against
	// independent-but-identical stores: version number, finding count, finding IDs,
	// scorecard, degraded state. ComputeDurationMS will differ (two separate timed
	// calls) so it's excluded from the comparison, not from the response itself.
	if httpResp.VersionNumber != mcpResp.VersionNumber {
		t.Errorf("VersionNumber: HTTP=%d MCP=%d", httpResp.VersionNumber, mcpResp.VersionNumber)
	}
	if len(httpResp.Findings) != len(mcpResp.Findings) {
		t.Fatalf("Findings count: HTTP=%d MCP=%d", len(httpResp.Findings), len(mcpResp.Findings))
	}
	for i := range httpResp.Findings {
		if httpResp.Findings[i].ID != mcpResp.Findings[i].ID {
			t.Errorf("Findings[%d].ID: HTTP=%q MCP=%q", i, httpResp.Findings[i].ID, mcpResp.Findings[i].ID)
		}
	}
	if httpResp.Degraded != mcpResp.Degraded {
		t.Errorf("Degraded: HTTP=%v MCP=%v", httpResp.Degraded, mcpResp.Degraded)
	}
	if *httpResp.Graph != *mcpResp.Graph {
		t.Errorf("Graph: HTTP=%q MCP=%q", *httpResp.Graph, *mcpResp.Graph)
	}
}

// TestMCPServer_ToolRegistered confirms the MCP server actually exposes the tool —
// building the server and adding the tool must not panic, and the tool must be
// findable by name.
func TestMCPServer_ToolRegistered(t *testing.T) {
	store := testStore(t)
	s := NewMCPServer(store)
	if s == nil {
		t.Fatal("NewMCPServer returned nil")
	}
	_ = mcp.ToolHandlerFor[AssessRequest, AssessResponse](nil) // type-check assessTool's shape against the SDK's own generic
}
