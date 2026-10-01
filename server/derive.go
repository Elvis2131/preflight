// PC-105: POST /canvas/derive — read-only, derived facts about a canvas document (each
// subnet's AZ, region, and route-derived public/private classification) for the workspace's
// Region/AZ groupings and public/private badge. It is a PURE derivation: nothing is stored,
// no session or version is created, and no verdict comes out of it — which is also why it
// works on a half-drawn design that /sessions/{id}/canvas would refuse (below the Minimum
// Viable Graph). The browser draws exactly what this returns and derives nothing itself.
package server

import (
	"encoding/json"
	"net/http"

	"preflight/core"
	"preflight/ingest"
)

// DeriveRequest is the body of POST /canvas/derive.
type DeriveRequest struct {
	Canvas core.CanvasDocument `json:"canvas"`
}

// DeriveResponse is the derived, read-only facts about a canvas document.
type DeriveResponse struct {
	Subnets []core.SubnetFact `json:"subnets"`
}

// DeriveCanvas computes the derived facts. Provider mappings are loaded exactly as
// AssessCanvas loads them (one registry, no second implementation).
func DeriveCanvas(req DeriveRequest) (DeriveResponse, error) {
	registry, err := loadMergedRegistry()
	if err != nil {
		return DeriveResponse{}, err
	}
	ir := ingest.DeriveCanvasIR(req.Canvas, registry)
	return DeriveResponse{Subnets: core.DeriveSubnetFacts(ir)}, nil
}

// DeriveCanvasHandler serves POST /canvas/derive.
func DeriveCanvasHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DeriveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}
		resp, err := DeriveCanvas(req)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
