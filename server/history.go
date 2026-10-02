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
	var costDelta []core.CostDeltaEntry
	var costSnapshotChanged bool
	if versionNumber > 1 {
		prev, prevOK, err := store.GetVersion(sessionID, versionNumber-1)
		if err != nil {
			return AssessResponse{}, err
		}
		if prevOK {
			prov := core.NewProvenance(core.KindDerived, "server:history:assurance-delta")
			delta = core.ComputeDelta(prev.Scorecard.StatusMap(), stored.Scorecard.StatusMap(), prov)

			costProv := core.NewProvenance(core.KindDerived, "server:history:cost-delta")
			costDelta, costSnapshotChanged = core.ComputeCostDelta(prev.Cost, stored.Cost, costProv)
		}
	}

	// Graph (PC-81): regenerated from the stored IR, not persisted separately — since
	// core.RenderDOT + render.SVG are proven deterministic (render/render_golden_test.go),
	// re-rendering here is guaranteed to reproduce byte-identical output to what the
	// original /assess call returned, at the cost of one cheap Graphviz invocation
	// per read-back rather than a second stored copy to keep in sync.
	graph := graphOrFailureMessage(stored.IR)
	return AssessResponse{
		SessionID:      sessionID,
		VersionNumber:  versionNumber,
		Findings:       stored.Findings,
		Scorecard:      stored.Scorecard,
		AssuranceDelta: delta,
		Graph:          &graph,
		Degraded:       true,
		DegradedReason: "read back from a previously-stored version; LLM narratives are not part of this response — stored ones are served by GET /sessions/{id}/versions/{n}/annotations",
		// ComputeDurationMS measures the actual store read (real, if tiny — a SQLite
		// row fetch, not an ingest/analyse pipeline), never a fabricated 0 standing
		// in for "not applicable": this genuinely IS the time this call took.
		ComputeDurationMS:   time.Since(start).Milliseconds(),
		Cost:                stored.Cost,
		CostDelta:           costDelta,
		CostSnapshotChanged: costSnapshotChanged,
	}, nil
}

// ListVersions (PC-92 groundwork) returns every version 1..latest for a session, each
// exactly what GetStoredVersion would return for that one version — reused directly,
// not reimplemented, so a caller building a timeline (per-dimension scorecard
// trajectory, PC-92's own card) gets the identical AssuranceDelta computation the
// single-version read-back already has, in one call instead of N sequential ones. A
// real gap found starting on PC-92: neither GetStoredVersion nor anything else could
// answer "how many versions does this session even have" — a CLI/one-shot caller
// never needed that, only "the latest" or "one specific number" (LatestVersion,
// GetVersion) — the same root pattern as every prior browser-surfaced API gap this
// session found (node_loss, CORS, inline workload, ...). A session that exists but
// has zero stored versions (EnsureSession succeeded, then the assessment itself
// failed before StoreVersion — e.g. an invalid workload) returns a real empty list,
// not an error: the session is real, it simply has no history yet.
func ListVersions(store *Store, sessionID string) ([]AssessResponse, error) {
	exists, err := store.SessionExists(sessionID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, newAPIError(http.StatusNotFound, "session_not_found", "server: no session %s found", sessionID)
	}

	latest, hadAny, err := store.LatestVersion(sessionID)
	if err != nil {
		return nil, err
	}
	if !hadAny {
		return []AssessResponse{}, nil
	}

	out := make([]AssessResponse, 0, latest.VersionNumber)
	for n := 1; n <= latest.VersionNumber; n++ {
		resp, err := GetStoredVersion(store, sessionID, n)
		if err != nil {
			return nil, err
		}
		out = append(out, resp)
	}
	return out, nil
}

// ListVersionsHandler serves GET /sessions/{id}/versions (PC-92 groundwork) — no
// trailing {n}, a distinct route from GetVersionHandler's own pattern.
func ListVersionsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")

		resp, err := ListVersions(store, sessionID)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
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
