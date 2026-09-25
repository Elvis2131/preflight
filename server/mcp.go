package server

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"preflight/core"
)

// NewMCPServer builds the MCP server exposing "assess", "simulate", and
// "assess_canvas" — the other half of PC-21's "MCP tools and HTTP routes must call
// identical underlying functions" requirement. Each *Tool below and its HTTP
// counterpart (http.go) both call the same Assess/Simulate/AssessCanvas function;
// neither reimplements any part of it.
//
// PC-93's fix, and the reasoning behind it (recorded here per this ticket's own
// first acceptance criterion): mcp.AddTool's generic form reflects a tool's In/Out
// struct tags via google/jsonschema-go when — and ONLY when — Tool.InputSchema/
// OutputSchema is nil at call time (confirmed by reading go-sdk v1.0.0's own
// setSchema, mcp/server.go — the reflection branch is gated on `*sfield == nil`,
// the else branch just re-marshals whatever was already provided through plain
// encoding/json, agnostic to its concrete Go type). That reflection library parses
// the `jsonschema:` struct tag completely differently than invopop/jsonschema does
// (confirmed by reading google/jsonschema-go v0.3.0's own infer.go): it does not
// parse `required`/`minLength=1` as separate constraint keywords at all — the ENTIRE
// tag value becomes the field's Description string, required-ness is derived instead
// from the JSON `omitempty`/`omitzero` tag, and any tag content containing a bare
// `key=value` pattern (like our own `minLength=1`) is REJECTED outright by a
// deliberate guard (disallowedPrefixRegexp) — not a bug in that library, a real,
// permanent incompatibility with how core/'s own tags are written for
// invopop/jsonschema (contracts/*.schema.json's own generator, cmd/gen-contracts).
//
// Given that, none of PC-93's own three originally-listed options were actually the
// best fix: manually decoding raw arguments (still correct, but unnecessary extra
// work per tool) and patching the tag reflection (wouldn't even produce a CORRECT
// schema — the library still can't express required/minLength as real schema
// keywords, only as description text) are both strictly worse than simply supplying
// Tool.InputSchema/OutputSchema ourselves, reflected via the SAME invopop reflector
// server/openapi.go's reflectSchema already uses for contracts/*.schema.json and the
// OpenAPI spec — one schema-generation mechanism for this whole codebase, not two,
// and core/'s frozen contract tags stay exactly as they are.
func NewMCPServer(store *Store) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "preflight", Title: "Preflight architecture assurance"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:         "assess",
		Description:  "Run a deterministic architecture assessment against a Terraform bundle and workload declaration, returning findings, a scorecard, and (from a session's second call onward) an Assurance Delta against the previous version.",
		OutputSchema: reflectSchema(AssessResponse{}),
	}, assessTool(store))
	mcp.AddTool(s, &mcp.Tool{
		Name:         "simulate",
		Description:  "Declare a single fault (region_loss or node_loss) against an already-assessed version and get back severed paths, cascade, surviving capacity, and a verdict — without re-running the full /assess pipeline. PC-82, PC-88.",
		OutputSchema: reflectSchema(core.SimulateResponse{}),
	}, simulateTool(store))
	mcp.AddTool(s, &mcp.Tool{
		Name:         "assess_canvas",
		Description:  "Assess a canvas-authored architecture (PC-85/86) — builds an IR directly from a posted CanvasDocument rather than parsing Terraform, then runs the identical downstream pipeline as \"assess\". PC-93: previously missing because its input type's struct tags panicked the SDK's default schema reflection.",
		InputSchema:  reflectSchema(AssessCanvasRequest{}),
		OutputSchema: reflectSchema(AssessResponse{}),
	}, assessCanvasTool(store))
	mcp.AddTool(s, &mcp.Tool{
		Name:         "trace",
		Description:  "Trace one request's path through an already-assessed architecture — route selection, Network ACLs at each subnet boundary, Security Groups at the destination, and a structural target-health check — returning an ordered, provenance-tagged, explainable step list plus a concise allow/deny verdict. PC-114.",
		InputSchema:  reflectSchema(TraceRequest{}),
		OutputSchema: reflectSchema(core.Trace{}),
	}, traceTool(store))
	return s
}

func assessTool(store *Store) mcp.ToolHandlerFor[AssessRequest, AssessResponse] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input AssessRequest) (*mcp.CallToolResult, AssessResponse, error) {
		resp, err := Assess(store, input)
		if err != nil {
			return nil, AssessResponse{}, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, AssessResponse{}, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(buf)}},
			StructuredContent: resp,
		}, resp, nil
	}
}

// simulateTool is /simulate's MCP counterpart (PC-82) — same thin-wrapper discipline
// as assessTool, for the same reason (see NewMCPServer's own doc comment).
func simulateTool(store *Store) mcp.ToolHandlerFor[SimulateRequest, core.SimulateResponse] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input SimulateRequest) (*mcp.CallToolResult, core.SimulateResponse, error) {
		resp, err := Simulate(store, input)
		if err != nil {
			return nil, core.SimulateResponse{}, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, core.SimulateResponse{}, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(buf)}},
			StructuredContent: resp,
		}, resp, nil
	}
}

// assessCanvasTool is /sessions/{id}/canvas's MCP counterpart (PC-86/PC-93) — same
// thin-wrapper discipline as assessTool. Unlike the HTTP route, there is no URL path
// to take SessionID from, so a real caller must supply it in the tool arguments
// (AssessCanvasRequest.SessionID carries a real json tag for exactly this reason —
// see that field's own doc comment in server/canvas.go).
func assessCanvasTool(store *Store) mcp.ToolHandlerFor[AssessCanvasRequest, AssessResponse] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input AssessCanvasRequest) (*mcp.CallToolResult, AssessResponse, error) {
		resp, err := AssessCanvas(store, input)
		if err != nil {
			return nil, AssessResponse{}, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, AssessResponse{}, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(buf)}},
			StructuredContent: resp,
		}, resp, nil
	}
}

// traceTool is /sessions/{id}/trace's MCP counterpart (PC-114) — same thin-wrapper
// discipline as assessCanvasTool: no URL path here either, so a real caller supplies
// SessionID directly in the tool arguments.
func traceTool(store *Store) mcp.ToolHandlerFor[TraceRequest, core.Trace] {
	return func(_ context.Context, _ *mcp.CallToolRequest, input TraceRequest) (*mcp.CallToolResult, core.Trace, error) {
		resp, err := Trace(store, input)
		if err != nil {
			return nil, core.Trace{}, err
		}

		buf, err := json.Marshal(resp)
		if err != nil {
			return nil, core.Trace{}, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: string(buf)}},
			StructuredContent: resp,
		}, resp, nil
	}
}
