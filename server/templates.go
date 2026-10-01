// PC-108: GET /templates and GET /templates/{id} expose the reference-architecture
// template library (golden/templates) to the workspace. These are pure reads of
// checked-in DATA: loading a template into the canvas and assessing it goes through the
// ordinary POST /sessions/{id}/canvas path — there is deliberately no "assess template"
// endpoint, so no template-specific code path can exist.
package server

import (
	"encoding/json"
	"net/http"

	"preflight/golden/templates"
)

// ListTemplatesHandler serves GET /templates.
func ListTemplatesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := templates.List()
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

// GetTemplateHandler serves GET /templates/{id}: the template's canvas document,
// workload, and UI-only layout.
func GetTemplateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ids, err := templates.IDs()
		if err != nil {
			writeError(w, err)
			return
		}
		found := false
		for _, known := range ids {
			if known == id {
				found = true
			}
		}
		if !found {
			writeError(w, newAPIError(http.StatusNotFound, "template_not_found", "server: no template %q", id))
			return
		}
		t, err := templates.Load(id)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	}
}
