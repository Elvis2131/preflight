// This file serves assessd's own API description: GET /openapi.json (the machine-
// readable spec) and GET /swagger (a browser UI over it). Per this project's own PC-7
// discipline ("each schema is generated from the same Go structs the code validates
// against — not hand-written JSON", cmd/gen-contracts/main.go), the request/response
// schemas embedded here are reflected from AssessRequest/AssessResponse themselves via
// invopop/jsonschema — the same library cmd/gen-contracts already depends on — not
// retyped by hand into a separate document that could silently drift from the real
// wire shape.
//
// Scope: this describes assessd (P1)'s own real HTTP routes (/healthz, /assess,
// /simulate). The MCP tools (mcp.go) are a separate transport with their own schema
// mechanism
// (the MCP go-sdk's tool-call schema) and is not re-described here — documenting the
// same operation twice, in two schema dialects that could drift, would be exactly the
// duplication risk PC-21's own Conversation already named for the HTTP/MCP split.
package server

import (
	"encoding/json"
	"net/http"
	"reflect"

	"preflight/core"

	"github.com/invopop/jsonschema"
)

// reflectSchema reflects one Go type into a fully self-contained JSON Schema — no
// $defs/$ref indirection (DoNotReference: true) — so the result can be embedded
// directly into the OpenAPI document at any nesting depth without a separate
// components/schemas registry to resolve refs against.
func reflectSchema(v any) *jsonschema.Schema {
	r := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
	return r.ReflectFromType(reflect.TypeOf(v))
}

// buildOpenAPISpec constructs the OpenAPI 3.1 document. 3.1 is used deliberately, not
// 3.0: 3.1's schema objects ARE JSON Schema 2020-12 (the same dialect
// cmd/gen-contracts already stamps on contracts/*.schema.json), so the reflected
// AssessRequest/AssessResponse schemas embed without any dialect translation.
func buildOpenAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Preflight assessd (P1) API",
			"description": "Deterministic assessment engine — sync, pure, fast. No credentials, no external calls (CLAUDE.md §7). Schemas below are reflected from this server's own Go request/response types, not hand-written.",
			"version":     "0.1.0",
		},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": map[string]any{
					"summary": "Liveness check",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "assessd is running",
							"content": map[string]any{
								"text/plain": map[string]any{
									"schema": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
			"/assess": map[string]any{
				"post": map[string]any{
					"summary":     "Run one deterministic assessment",
					"description": "ingest -> core engines (PC-14/17/18/28) -> scorecard -> assurance delta (if a prior version exists in this session) -> persist -> respond. Never calls reason/ — see AssessResponse.degraded.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": reflectSchema(AssessRequest{}),
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Assessment succeeded",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(AssessResponse{}),
								},
							},
						},
						"400": map[string]any{
							"description": "Malformed request body or missing session_id (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"422": map[string]any{
							"description": "A well-formed request that can't be honored: an unloadable/invalid workload (error_code: invalid_workload), an unparseable bundle (invalid_bundle), or the bundle is below the Minimum Viable Graph threshold (insufficient_model)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/simulate": map[string]any{
				"post": map[string]any{
					"summary":     "Declare a single fault against an already-assessed version",
					"description": "Reuses PC-14's existing fault-injection engines (SimulateLoss, SurvivingCapacity) against a version /assess already produced — no fresh ingest. Two fault types are hand-verified: region_loss (PC-82) and node_loss (PC-88); see core.Simulate's own doc comment.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": reflectSchema(SimulateRequest{}),
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Simulation succeeded (a not_assessable verdict for an unsupported fault type or target is still a 200 — it is a real, honest answer, not an error)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(core.SimulateResponse{}),
								},
							},
						},
						"400": map[string]any{
							"description": "Malformed request body, missing session_id, or version_number < 1 (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"404": map[string]any{
							"description": "No such session at all (error_code: session_not_found), or the session exists but not that version_number (error_code: version_not_found)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/sessions/{id}/versions": map[string]any{
				"get": map[string]any{
					"summary":     "List every version 1..latest for a session (PC-92 groundwork)",
					"description": "Each entry is exactly what GET /sessions/{id}/versions/{n} would return for that version — reused directly, not reimplemented, so a timeline caller gets the identical AssuranceDelta computation in one call instead of N sequential ones. A session that exists but has zero stored versions (an assessment failed before persisting) returns a real empty array, not an error.",
					"parameters": []any{
						map[string]any{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]any{"type": "string"},
							"description": "Session ID.",
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "A JSON array of AssessResponse, one per version, oldest first",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type":  "array",
										"items": reflectSchema(AssessResponse{}),
									},
								},
							},
						},
						"404": map[string]any{
							"description": "No such session (error_code: session_not_found)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/sessions/{id}/versions/{n}": map[string]any{
				"get": map[string]any{
					"summary":     "Re-fetch a previously-computed assessment (PC-94)",
					"description": "Loads Findings/Scorecard directly from storage — no re-ingest, no re-run of core.BuildFindings/BuildScorecard. The API-side half of canvas's own \"No persistence\" scope cut: a persistent, multi-turn client (a browser session) can now re-fetch a version's own result without re-POSTing the same bundle/canvas/workload again. Additive only — no change to /assess, /simulate, or /sessions/{id}/canvas's own behavior.",
					"parameters": []any{
						map[string]any{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]any{"type": "string"},
							"description": "Session ID.",
						},
						map[string]any{
							"name": "n", "in": "path", "required": true,
							"schema":      map[string]any{"type": "integer"},
							"description": "Version number to re-fetch.",
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Read-back succeeded — identical response shape to /assess",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(AssessResponse{}),
								},
							},
						},
						"400": map[string]any{
							"description": "The version number in the path isn't a valid integer (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"404": map[string]any{
							"description": "No such session at all (error_code: session_not_found), or the session exists but not that version_number (error_code: version_not_found)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/sessions/{id}/versions/{n}/report": map[string]any{
				"get": map[string]any{
					"summary":     "Assemble a structured report for a stored version (PC-120)",
					"description": "Projection only, per its own Card: every value traces to an existing engine's own output (BuildFindings, PC-121's compliance catalogs, PC-125/126's traffic/load, PC-117's cost, NFR conformance, an assumptions appendix) — this endpoint invents no new verdict, score, or simulation logic. Sections whose underlying data is unavailable for this version (e.g. no pricing snapshot was attached) render with an explicit unavailable_reason rather than being silently omitted.",
					"parameters": []any{
						map[string]any{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]any{"type": "string"},
							"description": "Session ID.",
						},
						map[string]any{
							"name": "n", "in": "path", "required": true,
							"schema":      map[string]any{"type": "integer"},
							"description": "Version number to report on.",
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Report assembled successfully",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(core.Report{}),
								},
							},
						},
						"400": map[string]any{
							"description": "The version number in the path isn't a valid integer (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"404": map[string]any{
							"description": "No such session at all (error_code: session_not_found), or the session exists but not that version_number (error_code: version_not_found)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/sessions/{id}/canvas": map[string]any{
				"post": map[string]any{
					"summary":     "Assess a canvas-authored architecture",
					"description": "A second IR producer, symmetric to /assess (PC-86): builds an IR directly from a posted CanvasDocument (PC-85's own frozen contract, contracts/canvas.schema.json) rather than parsing Terraform, then runs the identical downstream pipeline. A dangling edge or a node missing capability fields never crashes and never guesses — it produces real not_assessable results, the same guarantee /assess already has.",
					"parameters": []any{
						map[string]any{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]any{"type": "string"},
							"description": "Session ID — a new one starts Version 1; an existing one produces the next VersionNumber and a real AssuranceDelta against its predecessor.",
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": reflectSchema(AssessCanvasRequest{}),
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Assessment succeeded — identical response shape to /assess",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(AssessResponse{}),
								},
							},
						},
						"400": map[string]any{
							"description": "Malformed request body or missing session_id (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"422": map[string]any{
							"description": "A well-formed request that can't be honored: an invalid workload — file-based or inline (error_code: invalid_workload) — invalid VPC/subnet placement, naming each broken rule and resource (error_code: invalid_placement, PC-105) or invalid route/NACL input (error_code: invalid_network_controls, PC-138/139) — or the canvas is below the Minimum Viable Graph threshold (insufficient_model)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
			"/sessions/{id}/trace": map[string]any{
				"post": map[string]any{
					"summary":     "Trace one request's path through an already-assessed architecture (PC-114)",
					"description": "Runs route selection, Network ACLs at each subnet boundary, Security Groups at the destination, and a structural target-health check against an already-stored version's IR — an ordered, provenance-tagged, explainable step list plus a concise allow/deny/not_assessable verdict. destination is a required IR node ID; source is an IR node ID too, or omit it (with source_cidr set, e.g. \"0.0.0.0/0\") to mean an internet-originated request. A service type not yet modelled for request simulation produces a real not_assessable step naming it, never a guessed result.",
					"parameters": []any{
						map[string]any{
							"name": "id", "in": "path", "required": true,
							"schema":      map[string]any{"type": "string"},
							"description": "Session ID.",
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": reflectSchema(TraceRequest{}),
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "The trace was built — see its own Allowed field for the verdict; a denied or not_assessable outcome is still a 200, not an error",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(core.Trace{}),
								},
							},
						},
						"400": map[string]any{
							"description": "Missing session_id, version_number, destination, or protocol (error_code: invalid_request_body)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
						"404": map[string]any{
							"description": "No such session at all (error_code: session_not_found), or the session exists but not that version_number (error_code: version_not_found)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": reflectSchema(errorBody{}),
								},
							},
						},
					},
				},
			},
		},
	}
}

// OpenAPISpecHandler serves GET /openapi.json.
func OpenAPISpecHandler() http.HandlerFunc {
	spec := buildOpenAPISpec()
	buf, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		panic("server: failed to marshal the OpenAPI spec — this is a bug in buildOpenAPISpec, not a runtime condition: " + err.Error())
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
	}
}

// swaggerUIPage loads Swagger UI 5.29.1 from cdnjs (a real, currently-published
// version — checked against cdnjs' own package index before pinning, not guessed) to
// render /openapi.json. This is the one page assessd serves that reaches outside the
// process — and it does so only in the caller's own browser, at the caller's own
// request; assessd itself makes zero outbound calls to produce it (CLAUDE.md §7's "no
// external calls" is about the server process, not about what a human's browser tab
// chooses to load once handed this page).
const swaggerUIPage = `<!DOCTYPE html>
<html>
<head>
  <title>Preflight assessd API</title>
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.29.1/swagger-ui.min.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.29.1/swagger-ui-bundle.min.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: '/openapi.json',
        dom_id: '#swagger-ui',
      });
    };
  </script>
</body>
</html>
`

// SwaggerUIHandler serves GET /swagger.
func SwaggerUIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(swaggerUIPage))
	}
}
