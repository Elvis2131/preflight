// This file is PC-120's HTTP surface: GET /sessions/{id}/versions/{n}/report. Thin —
// GetReport does no computation of its own beyond loading the already-stored version
// (exactly GetStoredVersion's own store reads) and calling core.BuildReport, the same
// "server/ is the I/O adapter, core/ is the pure engine" split every other handler in
// this package already follows.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"preflight/core"
)

// GetReport loads versionNumber's own stored IR/Workload/Findings/Scorecard/Cost and
// the same AssuranceDelta GetStoredVersion already computes against the prior
// version, then calls core.BuildReport — no new engine call this package's other
// handlers don't already make, per the Card's own "projection only" instruction.
func GetReport(store *Store, sessionID string, versionNumber int) (core.Report, error) {
	exists, err := store.SessionExists(sessionID)
	if err != nil {
		return core.Report{}, err
	}
	if !exists {
		return core.Report{}, newAPIError(http.StatusNotFound, "session_not_found",
			"server: no session %s found", sessionID)
	}

	stored, ok, err := store.GetVersion(sessionID, versionNumber)
	if err != nil {
		return core.Report{}, err
	}
	if !ok {
		return core.Report{}, newAPIError(http.StatusNotFound, "version_not_found",
			"server: no version %d found for session %s", versionNumber, sessionID)
	}

	var delta []core.DeltaEntry
	if versionNumber > 1 {
		prev, prevOK, err := store.GetVersion(sessionID, versionNumber-1)
		if err != nil {
			return core.Report{}, err
		}
		if prevOK {
			prov := core.NewProvenance(core.KindDerived, "server:report:assurance-delta")
			delta = core.ComputeDelta(prev.Scorecard.StatusMap(), stored.Scorecard.StatusMap(), prov)
		}
	}

	return core.BuildReport(sessionID, versionNumber, stored.IR, stored.Workload, stored.Findings, stored.Scorecard, stored.Cost, delta), nil
}

// GetReportHandler serves GET /sessions/{id}/versions/{n}/report, registered with Go
// 1.22+'s pattern-matching ServeMux (see cmd/assessd/main.go) — same thin-wrapper
// discipline as GetVersionHandler.
func GetReportHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		versionNumber, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid version number in path: %s", err))
			return
		}

		report, err := GetReport(store, sessionID, versionNumber)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	}
}
