package server_test

// PC-93's own second acceptance criterion, verbatim: "assess_canvas ... registers
// without panicking and is exercised by a real MCP client call, not just a
// successful NewMCPServer() construction." TestMCPServer_ToolRegistered (in
// samefunction_test.go) already proves the first half. This proves the second: a
// REAL mcp.Client, connected over a REAL (in-memory) Transport, calling CallTool
// with JSON arguments that go through the actual wire-marshaling and schema
// validation path (applySchema, in the SDK's own server.go) — not just invoking the
// Go handler function directly, which would skip exactly the code path this ticket's
// whole root cause lived in.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"preflight/core"
	"preflight/server"
)

func TestMCPClient_CallAssessCanvasTool_RealClientServerRoundTrip(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	mcpServer := server.NewMCPServer(store)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverSession, err := mcpServer.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer serverSession.Wait()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer clientSession.Close()

	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// Real arguments, marshaled to real JSON by the client and unmarshaled/validated
	// by the real server-side schema path — the exact path that used to panic at
	// registration time before this fix, now exercised end to end at call time too.
	args := map[string]any{
		"session_id": "mcp-canvas-real-call-test",
		"canvas": core.CanvasDocument{
			Nodes: []core.CanvasNode{
				{ID: "dns1", Type: "dns", Label: "DNS", Capability: map[string]string{}},
				{ID: "db1", Type: "managed_database", Label: "DB", Capability: map[string]string{
					"encryption_mechanism": "true",
				}},
			},
			Edges: []core.CanvasEdge{
				{ID: "e1", Type: "routes_to", From: "dns1", To: "db1"},
			},
		},
		"workload_path": workloadPath,
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "assess_canvas",
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(\"assess_canvas\"): %v", err)
	}
	if result.IsError {
		t.Fatalf("assess_canvas tool returned an error result: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected real content in the tool result")
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("result.Content[0] type = %T, want *mcp.TextContent", result.Content[0])
	}
	var resp server.AssessResponse
	if err := json.Unmarshal([]byte(textContent.Text), &resp); err != nil {
		t.Fatalf("decode tool result text as AssessResponse: %v", err)
	}
	if resp.SessionID != "mcp-canvas-real-call-test" {
		t.Errorf("SessionID = %q, want mcp-canvas-real-call-test", resp.SessionID)
	}
	if len(resp.Findings) == 0 {
		t.Fatal("expected real findings from the canvas-authored managed_database node")
	}
}
