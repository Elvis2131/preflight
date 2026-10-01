// PC-131: saved Failure Lab scenarios. The store keeps DEFINITIONS only (a name and its
// declared faults); results are always recomputed by core.Simulate against the version
// asked about — never stored, never replayed — so a report reflects the current design.
package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"preflight/core"
)

// SaveScenario upserts one scenario definition for a session (creating the session row
// if needed — a scenario may be saved before anything has been assessed).
func (s *Store) SaveScenario(sessionID string, sc core.SavedScenario) error {
	if err := s.EnsureSession(sessionID); err != nil {
		return err
	}
	b, err := json.Marshal(sc.Faults)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO scenarios (session_id, name, faults_json) VALUES (?, ?, ?)
		ON CONFLICT(session_id, name) DO UPDATE SET faults_json = excluded.faults_json`, sessionID, sc.Name, string(b))
	return err
}

// ListScenarios returns a session's saved scenarios sorted by name (NFR-1). Never nil.
func (s *Store) ListScenarios(sessionID string) ([]core.SavedScenario, error) {
	rows, err := s.db.Query(`SELECT name, faults_json FROM scenarios WHERE session_id = ? ORDER BY name`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.SavedScenario{}
	for rows.Next() {
		var name, faults string
		if err := rows.Scan(&name, &faults); err != nil {
			return nil, err
		}
		sc := core.SavedScenario{Name: name}
		if err := json.Unmarshal([]byte(faults), &sc.Faults); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// DeleteScenario removes one definition; reports whether it existed.
func (s *Store) DeleteScenario(sessionID, name string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM scenarios WHERE session_id = ? AND name = ?`, sessionID, name)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func validScenario(sc core.SavedScenario) error {
	if err := validateScenarioShape(sc); err != nil {
		return newAPIError(http.StatusBadRequest, "invalid_request_body", "server: %s", err)
	}
	return nil
}

func validateScenarioShape(sc core.SavedScenario) error {
	name := strings.TrimSpace(sc.Name)
	if name == "" || len(name) > 80 {
		return errString("scenario name must be 1-80 characters")
	}
	if len(sc.Faults) == 0 {
		return errString("a scenario needs at least one fault")
	}
	for i, f := range sc.Faults {
		if f.Type == "" || f.Target == "" {
			return errString("fault " + strconv.Itoa(i+1) + " needs a type and a target")
		}
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

// SaveScenarioHandler serves PUT /sessions/{id}/scenarios/{name} with body {"faults":[...]}.
func SaveScenarioHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, err := url.PathUnescape(r.PathValue("name"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid scenario name: %s", err))
			return
		}
		var body struct {
			Faults []core.Fault `json:"faults"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}
		sc := core.SavedScenario{Name: strings.TrimSpace(name), Faults: body.Faults}
		if err := validScenario(sc); err != nil {
			writeError(w, err)
			return
		}
		if err := store.SaveScenario(r.PathValue("id"), sc); err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sc)
	}
}

// ListScenariosHandler serves GET /sessions/{id}/scenarios — definitions only.
func ListScenariosHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListScenarios(r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

// DeleteScenarioHandler serves DELETE /sessions/{id}/scenarios/{name}.
func DeleteScenarioHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, err := url.PathUnescape(r.PathValue("name"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid scenario name: %s", err))
			return
		}
		ok, err := store.DeleteScenario(r.PathValue("id"), name)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			writeError(w, newAPIError(http.StatusNotFound, "scenario_not_found", "server: no scenario %q for session %s", name, r.PathValue("id")))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// EvaluateSavedScenarios re-runs every saved scenario of a session against one stored
// version, now. (A pure function of the stored version plus the saved definitions.)
func EvaluateSavedScenarios(store *Store, sessionID string, versionNumber int) ([]core.ScenarioResult, error) {
	stored, ok, err := store.GetVersion(sessionID, versionNumber)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, newAPIError(http.StatusNotFound, "version_not_found",
			"server: no version %d found for session %s", versionNumber, sessionID)
	}
	saved, err := store.ListScenarios(sessionID)
	if err != nil {
		return nil, err
	}
	return core.EvaluateScenarios(stored.IR, stored.Workload, saved, core.NewProvenance(core.KindDerived, "server:scenarios")), nil
}

// EvaluateScenariosHandler serves GET /sessions/{id}/versions/{n}/scenarios.
func EvaluateScenariosHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("n"))
		if err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid version number in path: %s", err))
			return
		}
		res, err := EvaluateSavedScenarios(store, r.PathValue("id"), n)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}
