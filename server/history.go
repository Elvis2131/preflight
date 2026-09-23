// This file is PC-94's core: GET /sessions/{id}/versions/{n} re-serves a
// previously-computed assessment WITHOUT recomputation — the API-side half of the
// same gap canvas/README.md's own "No persistence" scope cut already names. Surfaced
// during the proactive API audit run after PC-89 closed: /assess, /simulate, and
// /sessions/{id}/canvas are all POST-only, write/compute-then-respond — nothing reads
// a prior result back. A CLI/agent caller is one-shot by nature and never needed
// this; a persistent, multi-turn browser session does, the same root pattern as
// node_loss/CORS/inline-workload/nil-slice-to-JSON-null/generic-400s before it.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"preflight/core"
)

// GetStoredVersion loads VersionNumber's already-persisted Findings/Scorecard
// directly from storage — no ingest, no core.BuildFindings/BuildScorecard re-run.
// The only derivation performed here is AssuranceDelta, and only as a diff between
// two ALREADY-STORED scorecards (this version and version-1, sequential by
// construction — see nextVersion's own doc comment) — comparing two stored results
// is not re-running the assessment itself, the same distinction assessFromResult's
// own delta computation already draws for the write path.
func GetStoredVersion(store *Store, sessionID string, versionNumber int) (AssessResponse, error) {
	start := time.Now()

	exists, err := store.SessionExists(sessionID)
	if err != nil {
		return AssessResponse{}, err
	}
	if !exists {
		return AssessResponse{}, newAPIError(http.StatusNotFound, "session_not_found",
			"server: no session %s found", sessionID)
	}

	stored, ok, err := store.GetVersion(sessionID, versionNumber)
	if err != nil {
		return AssessResponse{}, err
	}
	if !ok {
		return AssessResponse{}, newAPIError(http.StatusNotFound, "version_not_found",
			"server: no version %d found for session %s", versionNumber, sessionID)
	}

	var delta []core.DeltaEntry
	if versionNumber > 1 {
		prev, prevOK, err := store.GetVersion(sessionID, versionNumber-1)
		if err != nil {
			return AssessResponse{}, err
		}
		if prevOK {
			prov := core.NewProvenance(core.KindDerived, "server:history:assurance-delta")
			delta = core.ComputeDelta(prev.Scorecard.StatusMap(), stored.Scorecard.StatusMap(), prov)
		}
	}

	graphStub := "graph rendering not yet implemented — see PC-81"
	return AssessResponse{
		SessionID:      sessionID,
		VersionNumber:  versionNumber,
		Findings:       stored.Findings,
		Scorecard:      stored.Scorecard,
		AssuranceDelta: delta,
		Graph:          &graphStub,
		Degraded:       true,
		DegradedReason: "read back from a previously-stored version — reason/ was never re-run for a GET (ADR-005/PC-77's own gap, unchanged by this read-back)",
		// ComputeDurationMS measures the actual store read (real, if tiny — a SQLite
		// row fetch, not an ingest/analyse pipeline), never a fabricated 0 standing
		// in for "not applicable": this genuinely IS the time this call took.
		ComputeDurationMS: time.Since(start).Milliseconds(),
	}, nil
}

// GetVersionHandler serves GET /sessions/{id}/versions/{n} (PC-94), registered with
// Go 1.22+'s pattern-matching ServeMux (see cmd/assessd/main.go) — same thin-wrapper
// discipline as every other handler in http.go.
func GetVersionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		versionNumber, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid version number in path: %s", err))
			return
		}

		resp, err := GetStoredVersion(store, sessionID, versionNumber)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
