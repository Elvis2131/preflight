package server

import (
	"encoding/json"
	"errors"
	"net/http"
)

// errorBody is PC-95's wire shape for every structured failure response: a stable
// Code a caller can switch on, plus the same human-readable Message this codebase's
// plain errors already produced (so grepping response text still works unchanged for
// an existing CLI/test caller — additive, not breaking).
type errorBody struct {
	Code    string `json:"error_code"`
	Message string `json:"message"`
}

// writeError responds with a structured {error_code, message} body and the right
// HTTP status when err is an *APIError; otherwise it falls back to the same generic
// 400-with-plain-text behavior every handler in this file used before PC-95, so any
// error site not yet classified into an APIError degrades to exactly today's
// behavior rather than a broken or misleading response.
func writeError(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(apiErr.Status)
		json.NewEncoder(w).Encode(errorBody{Code: apiErr.Code, Message: apiErr.Message})
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// AssessHandler returns an http.HandlerFunc for POST /assess. It is a thin wrapper —
// decode request, call Assess, encode response — with zero assessment logic of its
// own, per this ticket's own Conversation: "MCP tools and HTTP routes must call
// identical underlying functions." See mcp.go's AssessTool for the other wrapper
// around the exact same Assess call.
func AssessHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req AssessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}

		resp, err := Assess(store, req)
		if err != nil {
			writeError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			// Headers are already sent at this point; nothing more to do but log.
			// A real logging setup is out of this ticket's scope (no logging story
			// exists yet) — stated as a real, current limitation, not silently absent.
			return
		}
	}
}

// SimulateHandler returns an http.HandlerFunc for POST /simulate (PC-82) — the same
// thin-wrapper discipline as AssessHandler: decode, call Simulate, encode, no logic
// of its own.
func SimulateHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req SimulateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}

		resp, err := Simulate(store, req)
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

// AssessCanvasHandler returns an http.HandlerFunc for POST /sessions/{id}/canvas
// (PC-86) — registered with Go 1.22+'s pattern-matching ServeMux (see
// cmd/assessd/main.go), so SessionID comes from the URL path via r.PathValue, not the
// JSON body. Same thin-wrapper discipline as every other handler in this file.
func AssessCanvasHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AssessCanvasRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, newAPIError(http.StatusBadRequest, "invalid_request_body", "invalid request body: %s", err.Error()))
			return
		}
		req.SessionID = r.PathValue("id")

		resp, err := AssessCanvas(store, req)
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

// WithCORS wraps a mux so the canvas frontend (canvas/, served from Vite's own dev
// origin — a different port than assessd's) can call assessd's HTTP routes directly
// from a browser (PC-88: the canvas's first-ever HTTP call, POST /sessions/{id}/canvas
// and POST /simulate for the "kill this node" interaction). Allow-Origin: * is
// deliberate, not a placeholder to tighten later: assessd holds no credentials and
// makes no external calls (CLAUDE.md §7's P1 definition) — there is no session cookie,
// bearer token, or secret a cross-origin page could exfiltrate by calling it, so the
// usual reason to restrict CORS (protecting an authenticated origin) doesn't apply
// here. A preflight OPTIONS request is answered directly, before it ever reaches the
// wrapped mux, since none of assessd's routes are registered for OPTIONS.
func WithCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
