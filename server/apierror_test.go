package server_test

// PC-95: proves the actual WIRE contract, not just the Go-level *server.APIError
// type — a real HTTP response, decoded from real JSON bytes, must carry the
// structured {error_code, message} body and the right status code. This is the same
// discipline TestSimulate_NoFaults_SeveredPathsAndJourneysAreNeverJSONNull already
// established for the nil-slice fix: a Go-level assertion (Code == "...") can pass
// while the actual bytes on the wire are wrong if nothing ever exercises the real
// HTTP handler + JSON encoding path.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"preflight/server"
)

type wireErrorBody struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func TestSimulateHandler_UnknownSession_RespondsWithStructured404(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	body, err := json.Marshal(server.SimulateRequest{SessionID: "never-assessed", VersionNumber: 1})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/simulate", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.SimulateHandler(store)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got wireErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if got.ErrorCode != "session_not_found" {
		t.Errorf("error_code = %q, want session_not_found", got.ErrorCode)
	}
	if got.Message == "" {
		t.Error("expected a non-empty message — an existing CLI caller greps this text")
	}
}

// TestSimulateHandler_MalformedJSON_RespondsWithStructured400 proves the OTHER real
// path through writeError — a plain (non-*APIError) decode failure still gets a
// stable error_code now (PC-95's own scope: distinguishing failure modes, malformed
// input included, not just the four cases named in the ticket's own Confirmation).
func TestSimulateHandler_MalformedJSON_RespondsWithStructured400(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	req := httptest.NewRequest(http.MethodPost, "/simulate", bytes.NewReader([]byte("{not json")))
	rec := httptest.NewRecorder()

	server.SimulateHandler(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var got wireErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response body is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if got.ErrorCode != "invalid_request_body" {
		t.Errorf("error_code = %q, want invalid_request_body", got.ErrorCode)
	}
}
