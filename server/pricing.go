// This file is PC-116/ADR-006's read-only half: GET /pricing/snapshots (list),
// GET /pricing/snapshots/{id} (metadata + entries), and POST
// /pricing/snapshots/{id}/activate (mark the active default) — all served by assessd
// (P1), reading pricing.Store directly. None of these make an AWS call; they only
// read/write already-fetched, already-normalized local data written by cmd/runnerd's
// own `-fetch-pricing` subcommand (ADR-006 §5) — pricing itself has zero network
// capability (pricing/boundary_test.go), so importing it here adds no network
// capability to P1 at all.
package server

import (
	"encoding/json"
	"net/http"

	"preflight/pricing"
)

// ListPricingSnapshotsHandler serves GET /pricing/snapshots.
func ListPricingSnapshotsHandler(store *pricing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListSnapshots()
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

// GetPricingSnapshotHandler serves GET /pricing/snapshots/{id}.
func GetPricingSnapshotHandler(store *pricing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		snap, ok, err := store.GetSnapshot(id)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			writeError(w, newAPIError(http.StatusNotFound, "snapshot_not_found", "server: no pricing snapshot %s found", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snap)
	}
}

// ActivatePricingSnapshotHandler serves POST /pricing/snapshots/{id}/activate.
func ActivatePricingSnapshotHandler(store *pricing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := store.SetActive(id); err != nil {
			writeError(w, newAPIError(http.StatusNotFound, "snapshot_not_found", "server: cannot activate pricing snapshot %s: %v", id, err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
