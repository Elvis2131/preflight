// This file is PC-114's server half: POST /sessions/{id}/trace, the same thin-wrapper
// discipline every other route in this package uses (decode, call core.BuildTrace,
// encode — no logic of its own) operating on an already-assessed, stored version, the
// same "no bundle_dir/workload_path here" reasoning as /simulate (PC-82): probing one
// request's path through an architecture that has already been assessed should never
// require re-running ingest.
package server

import (
	"encoding/json"
	"net/http"

	"preflight/core"
)

// TraceRequest is what a caller provides to trace one request's path — SessionID comes
// from the URL path (POST /sessions/{id}/trace), not this body, mirroring
// AssessCanvasRequest. Source is an IR node ID, or empty to mean "the internet", per
// core.BuildTrace's own documented convention — SourceCIDR names an external address
// only when Source is empty (an internal source's address, when needed at all, is
// derived from its own subnet's declared CIDR block, not supplied by the caller).
type TraceRequest struct {
	SessionID     string `json:"session_id,omitempty"`
	VersionNumber int    `json:"version_number"`
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	SourceCIDR    string `json:"source_cidr,omitempty"`
	Protocol      string `json:"protocol"`
	Port          int    `json:"port"`
}

// Trace loads a previously-assessed version's IR and runs core.BuildTrace against it —
// the same session/version-not-found distinction (PC-95) /simulate already
// established, reused verbatim rather than reinvented.
func Trace(store *Store, req TraceRequest) (core.Trace, error) {
	if req.SessionID == "" {
		return core.Trace{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: session_id is required")
	}
	if req.VersionNumber < 1 {
		return core.Trace{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: version_number must be >= 1")
	}
	if req.Destination == "" {
		return core.Trace{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: destination is required")
	}
	if req.Protocol == "" {
		return core.Trace{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: protocol is required")
	}

	exists, err := store.SessionExists(req.SessionID)
	if err != nil {
		return core.Trace{}, err
	}
	if !exists {
		return core.Trace{}, newAPIError(http.StatusNotFound, "session_not_found",
			"server: no session %s found — run /assess or POST /sessions/%s/canvas first", req.SessionID, req.SessionID)
	}

	stored, ok, err := store.GetVersion(req.SessionID, req.VersionNumber)
	if err != nil {
		return core.Trace{}, err
	}
	if !ok {
		return core.Trace{}, newAPIError(http.StatusNotFound, "version_not_found",
			"server: no version %d found for session %s — run /assess first", req.VersionNumber, req.SessionID)
	}

	return core.BuildTrace(stored.IR, req.Source, req.Destination, req.SourceCIDR, req.Protocol, req.Port), nil
}

// TraceHandler returns an http.HandlerFunc for POST /sessions/{id}/trace — registered
// with Go 1.22+'s pattern-matching ServeMux, same as AssessCanvasHandler.
func TraceHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req TraceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}
		req.SessionID = r.PathValue("id")

		resp, err := Trace(store, req)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			return
		}
	}
}
