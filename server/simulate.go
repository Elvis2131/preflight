// This file is PC-82's core: Simulate is the ONE function both the HTTP route and the
// MCP tool call — same discipline PC-21's own Conversation established for Assess
// ("MCP tools and HTTP routes must call identical underlying functions").
package server

import (
	"net/http"

	"preflight/core"
)

// SimulateRequest is what a caller provides to run one interactive fault simulation
// against an already-assessed version — PC-82's own stated shape: "{version,
// faults: [{type, target}]}". No bundle_dir/workload_path here, deliberately: this
// operates on a version /assess already produced (IR + Workload persisted together,
// see StoredVersion), not a fresh Terraform bundle — probing a "what if" should never
// require re-running the full ingest pipeline.
type SimulateRequest struct {
	SessionID     string       `json:"session_id"`
	VersionNumber int          `json:"version_number"`
	Faults        []core.Fault `json:"faults"`
}

// Simulate loads a previously-assessed version and applies the declared fault set
// against it via core.Simulate — the same fault-injection engine /assess's own
// findings already use internally (PC-14), not a duplicate.
func Simulate(store *Store, req SimulateRequest) (core.SimulateResponse, error) {
	if req.SessionID == "" {
		return core.SimulateResponse{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: session_id is required")
	}
	if req.VersionNumber < 1 {
		return core.SimulateResponse{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: version_number must be >= 1")
	}

	// PC-95: session-not-found and version-not-found are distinguished explicitly
	// (both 404, different Code) rather than collapsed into one "not found" message —
	// a caller that never assessed this session at all needs a different response
	// (start over) than one that assessed it but named the wrong version number
	// (retry with the right one).
	exists, err := store.SessionExists(req.SessionID)
	if err != nil {
		return core.SimulateResponse{}, err
	}
	if !exists {
		return core.SimulateResponse{}, newAPIError(http.StatusNotFound, "session_not_found",
			"server: no session %s found — run /assess or POST /sessions/%s/canvas first", req.SessionID, req.SessionID)
	}

	stored, ok, err := store.GetVersion(req.SessionID, req.VersionNumber)
	if err != nil {
		return core.SimulateResponse{}, err
	}
	if !ok {
		return core.SimulateResponse{}, newAPIError(http.StatusNotFound, "version_not_found",
			"server: no version %d found for session %s — run /assess first", req.VersionNumber, req.SessionID)
	}

	prov := core.NewProvenance(core.KindDerived, "server:simulate")
	return core.Simulate(stored.IR, stored.Workload, req.Faults, prov), nil
}
