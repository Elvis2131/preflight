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
	"preflight/render"
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

	// PC-132's own usage-based cost needs the exact PriceTable this version's own
	// Cost was computed from — re-fetched by the SAME snapshot ID CostReport already
	// carries (never the currently-active snapshot, which may have since changed),
	// mirroring computeCostIfAvailable's own row-conversion (server/assess.go).
	var priceTable core.PriceTable
	if stored.Cost != nil && store.pricingStore != nil {
		if snap, ok, err := store.pricingStore.GetSnapshot(stored.Cost.SnapshotID); err == nil && ok {
			priceTable.SnapshotID = snap.ID
			for _, e := range snap.Entries {
				priceTable.Rows = append(priceTable.Rows, core.PriceRow{
					Service: e.Service, Region: e.Region, SKUAttributes: e.SKUAttributes, Unit: e.Unit, Price: e.Price, Currency: e.Currency,
				})
			}
		}
	}

	graphSVG := graphOrFailureMessage(stored.IR)
	report := core.BuildReport(sessionID, versionNumber, graphSVG, stored.IR, stored.Workload, stored.Findings, stored.Scorecard, stored.Cost, priceTable, delta)

	// PC-131: saved Failure Lab scenarios, re-evaluated NOW against THIS version — the
	// report must reflect the current design, never a stored earlier result.
	saved, err := store.ListScenarios(sessionID)
	if err != nil {
		return core.Report{}, err
	}
	report = report.WithScenarios(stored.IR, stored.Workload, saved)

	// PC-154: LLM narratives, when generated and stored, as their own labelled section.
	if narratives, have, err := store.GetAnnotations(sessionID, versionNumber); err != nil {
		return core.Report{}, err
	} else if have {
		report = report.WithNarratives(narratives)
	}
	return report, nil
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

		// PC-122's own acceptance criterion, verbatim: "GET .../report?format=html|pdf
		// returns rendered output; JSON remains the default." Rendering itself is a
		// pure function of the already-built Report (core.RenderReportHTML) or a
		// subprocess call isolated in render/ (RenderPDF) — this handler adds no
		// content of its own, only picks which renderer to call.
		switch r.URL.Query().Get("format") {
		case "html":
			html, err := core.RenderReportHTML(report)
			if err != nil {
				writeError(w, newAPIError(http.StatusInternalServerError, "render_failed", "server: render report HTML: %v", err))
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(html))
			return
		case "pdf":
			html, err := core.RenderReportHTML(report)
			if err != nil {
				writeError(w, newAPIError(http.StatusInternalServerError, "render_failed", "server: render report HTML: %v", err))
				return
			}
			pdf, err := render.PDF(html)
			if err != nil {
				writeError(w, newAPIError(http.StatusInternalServerError, "render_failed", "server: render report PDF: %v", err))
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Write(pdf)
			return
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(report)
		}
	}
}
