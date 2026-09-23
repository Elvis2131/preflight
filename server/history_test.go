package server_test

// PC-94: proves the actual production code path (server.GetStoredVersion, the real
// HTTP handler) re-serves a previously-computed assessment without recomputation —
// not just that a Go-level function returns something.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"preflight/server"
)

func TestGetStoredVersion_FirstVersion_MatchesWhatAssessReturned(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	assessed, err := server.Assess(store, server.AssessRequest{
		SessionID: "history-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	readBack, err := server.GetStoredVersion(store, "history-test", 1)
	if err != nil {
		t.Fatalf("GetStoredVersion: %v", err)
	}

	if readBack.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", readBack.VersionNumber)
	}
	if len(readBack.Findings) != len(assessed.Findings) {
		t.Errorf("Findings count = %d, want %d (must match what /assess actually persisted, not a fresh recomputation)", len(readBack.Findings), len(assessed.Findings))
	}
	// Hand-verified: version 1 has no prior version, so no delta is possible —
	// same honesty rule assessFromResult's own write path already applies.
	if len(readBack.AssuranceDelta) != 0 {
		t.Errorf("AssuranceDelta = %+v, want empty for version 1", readBack.AssuranceDelta)
	}
}

func TestGetStoredVersion_SecondVersion_DeltaMatchesAssessTimeDelta(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	brokenDir, err := filepath.Abs("../golden/aws-broken")
	if err != nil {
		t.Fatal(err)
	}
	cleanDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "history-delta-test", BundleDir: brokenDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess v1: %v", err)
	}
	assessedV2, err := server.Assess(store, server.AssessRequest{
		SessionID: "history-delta-test", BundleDir: cleanDir, WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess v2: %v", err)
	}
	if len(assessedV2.AssuranceDelta) == 0 {
		t.Fatal("precondition failed: expected a real delta between broken and clean golden bundles")
	}

	readBack, err := server.GetStoredVersion(store, "history-delta-test", 2)
	if err != nil {
		t.Fatalf("GetStoredVersion: %v", err)
	}

	if len(readBack.AssuranceDelta) != len(assessedV2.AssuranceDelta) {
		t.Fatalf("AssuranceDelta has %d entries, want %d (recomputed from the same two STORED scorecards, must match exactly)",
			len(readBack.AssuranceDelta), len(assessedV2.AssuranceDelta))
	}
	// Compare every field EXCEPT Provenance.Source, which is deliberately different
	// (GetStoredVersion's own "server:history:assurance-delta" vs assessFromResult's
	// "server:assess:assurance-delta") — a real, correct distinction (this delta was
	// computed by the read-back path, not the write path), not something a read-back
	// should hide by faking assess-time's own source string.
	for i, want := range assessedV2.AssuranceDelta {
		got := readBack.AssuranceDelta[i]
		if got.FindingID != want.FindingID || got.Kind != want.Kind ||
			got.OldStatus != want.OldStatus || got.NewStatus != want.NewStatus || got.Reason != want.Reason {
			t.Errorf("AssuranceDelta[%d] = %+v, want %+v (ignoring Provenance.Source)", i, got, want)
		}
	}
}

func TestGetStoredVersion_UnknownSession_Returns404SessionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	_, err = server.GetStoredVersion(store, "never-existed", 1)
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "session_not_found" || apiErr.Status != http.StatusNotFound {
		t.Errorf("Code/Status = %q/%d, want session_not_found/404", apiErr.Code, apiErr.Status)
	}
}

func TestGetStoredVersion_KnownSessionUnknownVersion_Returns404VersionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "history-wrong-version-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	_, err = server.GetStoredVersion(store, "history-wrong-version-test", 99)
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "version_not_found" || apiErr.Status != http.StatusNotFound {
		t.Errorf("Code/Status = %q/%d, want version_not_found/404", apiErr.Code, apiErr.Status)
	}
}

// TestGetVersionHandler_RealHTTPRoundTrip proves the actual wire path — a real
// GET request through the real handler with real Go 1.22+ PathValue routing, decoded
// from real JSON bytes — not just the Go-level GetStoredVersion function.
func TestGetVersionHandler_RealHTTPRoundTrip(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "history-http-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sessions/history-http-test/versions/1", nil)
	req.SetPathValue("id", "history-http-test")
	req.SetPathValue("n", "1")
	rec := httptest.NewRecorder()

	server.GetVersionHandler(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var decoded struct {
		SessionID     string `json:"session_id"`
		VersionNumber int    `json:"version_number"`
		Findings      []any  `json:"findings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if decoded.SessionID != "history-http-test" || decoded.VersionNumber != 1 {
		t.Errorf("decoded = %+v, want session_id=history-http-test version_number=1", decoded)
	}
	if len(decoded.Findings) == 0 {
		t.Fatal("expected real findings in the read-back response")
	}
}
