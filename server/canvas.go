// This file is PC-86's server-side half: AssessCanvas is a second IR producer,
// symmetric to Assess (server/assess.go) — both share the exact same post-ingestion
// pipeline (assessFromResult) so the canvas path can never quietly diverge from the
// HCL path's own behavior, the same "MCP tools and HTTP routes must call identical
// underlying functions" discipline PC-21 established, extended here to a second
// producer rather than a second transport.
package server

import (
	"fmt"
	"net/http"
	"time"

	"preflight/core"
	"preflight/ingest"
)

// AssessCanvasRequest is what POST /sessions/{id}/canvas accepts. Over HTTP,
// SessionID is taken from the URL path (see AssessCanvasHandler) and overwrites
// whatever this field decoded to from the body — a REST-shaped, session-scoped
// resource, unlike /assess's flat JSON body, has no reason to duplicate the ID in
// both places. The field still carries a real json tag (not "-") specifically so the
// MCP tool path (assessCanvasTool, mcp.go) — which has no URL to take a path
// parameter from — can still supply it, the same way AssessRequest.SessionID already
// works for both transports.
type AssessCanvasRequest struct {
	SessionID string `json:"session_id,omitempty"`

	// Canvas is PC-85's own wire contract (contracts/canvas.schema.json) — the
	// browser posts exactly what canvas/src/serialize.ts produces.
	Canvas core.CanvasDocument `json:"canvas"`

	WorkloadPath string `json:"workload_path"`

	// Workload (PC-87) is an inline core.Workload — the NFR form's own output, posted
	// directly rather than as a server-filesystem path. A browser has no meaningful
	// way to hand the server a WorkloadPath (there is no client-writable path on
	// assessd's own disk that would mean anything), so a form-authored workload needs
	// this second, alternative field. Not a separate or looser schema: this is typed
	// as core.Workload itself and validated against the identical struct tags
	// contracts/workload.schema.json is generated from (PC-87's own acceptance
	// criterion: "no separate schema"). When both are absent, or Workload is present,
	// Workload wins — WorkloadPath remains for the Terraform-authored path (curl,
	// existing tests) where no form exists to have produced an inline one.
	Workload *core.Workload `json:"workload,omitempty"`
}

// AssessCanvas builds an IR directly from a posted canvas document
// (ingest.IngestCanvas — no HCL, no provider-mapping registry involved) and runs it
// through the identical downstream pipeline Assess uses. A deliberately incomplete
// canvas (a dangling edge, a node missing capability fields) never crashes and never
// gets a silent guess — it produces real not_assessable results, the same guarantee
// HCL ingestion already has (see ingest.IngestCanvas's own doc comment for exactly
// how).
func AssessCanvas(store *Store, req AssessCanvasRequest) (AssessResponse, error) {
	start := time.Now()

	if req.SessionID == "" {
		return AssessResponse{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: session_id is required")
	}

	versionNumber, hadPrev, prev, err := nextVersion(store, req.SessionID)
	if err != nil {
		return AssessResponse{}, err
	}

	workload, err := loadOrValidateWorkload(req.Workload, req.WorkloadPath)
	if err != nil {
		return AssessResponse{}, newAPIError(http.StatusUnprocessableEntity, "invalid_workload", "server: load workload: %s", err)
	}

	result, err := ingest.IngestCanvas(req.Canvas, versionNumber)
	if err != nil {
		return AssessResponse{}, fmt.Errorf("server: ingest canvas: %w", err)
	}

	return assessFromResult(store, req.SessionID, versionNumber, hadPrev, prev, workload, result, start)
}

// loadOrValidateWorkload resolves AssessCanvasRequest's two mutually-exclusive
// workload sources (see its own field doc comment for why both exist). An inline
// workload is validated with the exact same Workload.Validate() call
// ingest.LoadWorkload already uses for the file-based path — no separate, looser
// check for the form-authored case.
func loadOrValidateWorkload(inline *core.Workload, path string) (core.Workload, error) {
	if inline != nil {
		if err := inline.Validate(); err != nil {
			return core.Workload{}, fmt.Errorf("inline workload does not satisfy the frozen workload schema: %w", err)
		}
		return *inline, nil
	}
	return ingest.LoadWorkload(path)
}
