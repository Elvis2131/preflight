package server_test

// PC-92 groundwork: proves server.ListVersions — the real production code path a
// timeline viewer would call — against real, hand-verified data.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"preflight/server"
)

func TestListVersions_TwoVersions_MatchesIndividualReadBacks(t *testing.T) {
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
		SessionID: "list-versions-test", BundleDir: brokenDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess v1: %v", err)
	}
	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "list-versions-test", BundleDir: cleanDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess v2: %v", err)
	}

	list, err := server.ListVersions(store, "list-versions-test")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].VersionNumber != 1 || list[1].VersionNumber != 2 {
		t.Fatalf("version numbers = [%d, %d], want [1, 2]", list[0].VersionNumber, list[1].VersionNumber)
	}
	if len(list[0].AssuranceDelta) != 0 {
		t.Errorf("list[0].AssuranceDelta = %+v, want empty (version 1 has no prior)", list[0].AssuranceDelta)
	}
	if len(list[1].AssuranceDelta) == 0 {
		t.Fatal("expected a real delta on version 2 (broken -> clean golden bundle)")
	}

	// Cross-check against the single-version read-back this reuses — must match
	// exactly, since ListVersions is defined as calling GetStoredVersion in a loop,
	// not a separate implementation that could quietly diverge.
	individualV2, err := server.GetStoredVersion(store, "list-versions-test", 2)
	if err != nil {
		t.Fatalf("GetStoredVersion: %v", err)
	}
	if len(list[1].AssuranceDelta) != len(individualV2.AssuranceDelta) {
		t.Fatalf("ListVersions' v2 delta has %d entries, GetStoredVersion's has %d — must match",
			len(list[1].AssuranceDelta), len(individualV2.AssuranceDelta))
	}
	for i, want := range individualV2.AssuranceDelta {
		got := list[1].AssuranceDelta[i]
		if got.FindingID != want.FindingID || got.Kind != want.Kind {
			t.Errorf("AssuranceDelta[%d] = %+v, want %+v", i, got, want)
		}
	}
}

func TestListVersions_UnknownSession_Returns404SessionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	_, err = server.ListVersions(store, "never-existed")
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "session_not_found" || apiErr.Status != http.StatusNotFound {
		t.Errorf("Code/Status = %q/%d, want session_not_found/404", apiErr.Code, apiErr.Status)
	}
}

// TestListVersions_SessionExistsWithZeroVersions_ReturnsRealEmptyList is the honest
// edge case: a session can exist (EnsureSession succeeded) with zero stored versions
// if the assessment itself failed before StoreVersion — e.g. a bundle below the
// Minimum Viable Graph threshold. That is a real, different fact from "no such
// session" and must never be conflated with it.
func TestListVersions_SessionExistsWithZeroVersions_ReturnsRealEmptyList(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	insufficientDir, err := filepath.Abs("../ingest/testdata/insufficient-fixture")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}

	_, err = server.Assess(store, server.AssessRequest{
		SessionID: "zero-version-session", BundleDir: insufficientDir, WorkloadPath: workloadPath,
	})
	if err == nil {
		t.Fatal("expected the insufficient-fixture bundle to fail assessment (precondition)")
	}

	list, err := server.ListVersions(store, "zero-version-session")
	if err != nil {
		t.Fatalf("ListVersions must not error for a real session with zero versions, got: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("list = %v, want a real empty (non-nil) slice", list)
	}
}

func TestListVersionsHandler_RealHTTPRoundTrip(t *testing.T) {
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
		SessionID: "list-versions-http-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sessions/list-versions-http-test/versions", nil)
	req.SetPathValue("id", "list-versions-http-test")
	rec := httptest.NewRecorder()

	server.ListVersionsHandler(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not a valid JSON array: %v (body: %s)", err, rec.Body.String())
	}
	if len(raw) != 1 {
		t.Fatalf("len(raw) = %d, want 1", len(raw))
	}
	if raw[0]["version_number"] != float64(1) {
		t.Errorf("version_number = %v, want 1", raw[0]["version_number"])
	}
}
