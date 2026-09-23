package server_test

// PC-82: proves server.Simulate — the actual production code path — works against a
// real assessed version, not just core.Simulate in isolation (core/simulate_golden_test.go
// covers that). Confirms the Workload persisted by Assess (this session's own fix —
// StoredVersion didn't carry Workload at all before) round-trips correctly into
// Simulate without the caller re-supplying it.

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/server"
)

func TestSimulate_AgainstARealAssessedVersion_RegionLoss(t *testing.T) {
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

	assessResp, err := server.Assess(store, server.AssessRequest{
		SessionID:    "simulate-test",
		BundleDir:    bundleDir,
		WorkloadPath: workloadPath,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}

	simResp, err := server.Simulate(store, server.SimulateRequest{
		SessionID:     "simulate-test",
		VersionNumber: assessResp.VersionNumber,
		Faults:        []core.Fault{{Type: "region_loss", Target: "eu-west-1"}},
	})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	if simResp.Verdict.Value != "total_outage" {
		t.Errorf("Verdict.Value = %v, want total_outage", simResp.Verdict.Value)
	}
	if len(simResp.SeveredPaths) == 0 {
		t.Error("expected real severed paths against the golden AWS bundle's stateful nodes")
	}
}

// TestSimulate_UnknownSession_Returns404SessionNotFound (PC-95) is the precise
// version of what used to be a single coarse "some error happened" check: a session
// that was NEVER assessed (never even EnsureSession'd) must come back as a real
// *server.APIError with Code "session_not_found" and Status 404 — not just any
// non-nil error, and not the "version_not_found" code (a different, distinguishable
// fact: "this session exists" vs "this session never existed at all").
func TestSimulate_UnknownSession_Returns404SessionNotFound(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	_, err = server.Simulate(store, server.SimulateRequest{
		SessionID: "never-assessed", VersionNumber: 1,
		Faults: []core.Fault{{Type: "region_loss", Target: "eu-west-1"}},
	})
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "session_not_found" {
		t.Errorf("Code = %q, want session_not_found", apiErr.Code)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusNotFound)
	}
}

// TestSimulate_SessionExistsButVersionDoesnt_Returns404VersionNotFound (PC-95) is the
// OTHER not-found case: a real session that HAS been assessed, but a version number
// that doesn't exist for it — distinguishable from session_not_found by Code, both
// still 404 (both are "the thing you asked for doesn't exist"), never mistaken for
// each other by only inspecting the status code.
func TestSimulate_SessionExistsButVersionDoesnt_Returns404VersionNotFound(t *testing.T) {
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
		SessionID: "real-session-wrong-version", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	_, err = server.Simulate(store, server.SimulateRequest{
		SessionID: "real-session-wrong-version", VersionNumber: 99,
		Faults: []core.Fault{{Type: "region_loss", Target: "eu-west-1"}},
	})
	var apiErr *server.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want a *server.APIError", err)
	}
	if apiErr.Code != "version_not_found" {
		t.Errorf("Code = %q, want version_not_found — the session itself DOES exist", apiErr.Code)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusNotFound)
	}
}
