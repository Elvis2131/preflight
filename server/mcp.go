package server

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPServer builds the MCP server exposing the "assess" tool — the other half of
// PC-21's "MCP tools and HTTP routes must call identical underlying functions"
// requirement. assessTool below and AssessHandler (http.go) both call Assess and
// nothing else; neither reimplements any part of it.
//
// Out is `any`, not AssessResponse, deliberately — found the hard way: mcp.AddTool's
// generic form reflects the Out type's struct tags via google/jsonschema-go to build
// an output schema, and that library parses the `jsonschema:` tag differently than
// invopop/jsonschema does (the library contracts/*.schema.json is correctly generated
// with — see contracts_test's own passing validation against those exact tags).
// google/jsonschema-go panics on the mix of a bare `required` keyword with
// `minLength=1` in the same tag that invopop/jsonschema treats as entirely valid.
// Rather than weaken core/'s frozen contract tags to satisfy a second, incompatible
// schema library, this tool uses `any` for Out (per mcp.AddTool's own documented
// behavior: "If the Out type is any, the output schema is omitted") and populates the
// result content manually.
//
// PC-86 hit the same root cause on the INPUT side (AssessCanvasRequest, see below) —
// two independent occurrences of one incompatibility. PC-93 owns fixing this properly
// (a real decision between: manually decoding raw tool arguments for any frozen-contract
// input type, patching the tag reflection, or normalizing core/'s own jsonschema tags to
// a syntax both libraries accept) rather than each new tool re-discovering and
// re-documenting the same workaround.
func NewMCPServer(store *Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "preflight", Title: "Preflight architecture assurance"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "assess",
		Description: "Run a deterministic architecture assessment against a Terraform bundle and workload declaration, returning findings, a scorecard, and (from a session's second call onward) an Assurance Delta against the previous version.",
	}, assessTool(store))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "simulate",
		Description: "Declare a single fault (region_loss or node_loss) against an already-assessed version and get back severed paths, cascade, surviving capacity, and a verdict — without re-running the full /assess pipeline. PC-82, PC-88.",
	}, simulateTool(store))
	// No "assess_canvas" MCP tool, deliberately: mcp.AddTool[AssessCanvasRequest, _]
	// panics at registration time — core.CanvasDocument's own fields use
	// jsonschema:"required,minLength=1", the same tag combination this file's own doc
	// comment above already found google/jsonschema-go cannot parse, previously only
	// hit and worked around on the OUTPUT side (Out any). This is PC-93 now — filed
	// once this became a second independent occurrence of the same root cause, not a
	// one-off. The HTTP endpoint (AssessCanvasHandler, http.go) is fully working and
	// verified live; only the MCP tool is missing pending PC-93.
	return s
}

func assessTool(store *Store) mcp.ToolHandlerFor[AssessRequest, any] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input AssessRequest) (*mcp.CallToolResult, any, error) {
		resp, err := Assess(store, input)
		if err != nil {
			return nil, nil, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
		}, resp, nil
	}
}

// simulateTool is /simulate's MCP counterpart (PC-82) — same Out-any workaround as
// assessTool, for the same reason (see NewMCPServer's own doc comment).
func simulateTool(store *Store) mcp.ToolHandlerFor[SimulateRequest, any] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input SimulateRequest) (*mcp.CallToolResult, any, error) {
		resp, err := Simulate(store, input)
		if err != nil {
			return nil, nil, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
		}, resp, nil
	}
}

