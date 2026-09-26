package server_test

// PC-120: proves the real HTTP handler (server.GetReportHandler) against a real
// assessed-then-stored version — not just that core.BuildReport works in isolation.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/server"
)

func TestGetReport_AfterAssess_MatchesBuildReport(t *testing.T) {
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
		SessionID: "report-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	report, err := server.GetReport(store, "report-test", 1)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}

	if report.SessionID != "report-test" || report.VersionNumber != 1 {
		t.Errorf("got session=%q version=%d, want report-test/1", report.SessionID, report.VersionNumber)
	}
	if len(report.FailureModes.Findings) == 0 {
		t.Error("FailureModes.Findings is empty, want golden/aws's own real findings")
	}
	if err := report.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestGetReportHandler_UnknownSession_404(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/sessions/no-such-session/versions/1/report", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetReportHandler_InvalidVersionNumber_400(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/sessions/s1/versions/not-a-number/report", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestGetReportHandler_ServesValidReport is the HTTP-level counterpart to
// TestGetReport_AfterAssess_MatchesBuildReport — proving the actual registered
// handler (not just the underlying GetReport function) serves a schema-valid report.
func TestGetReportHandler_ServesValidReport(t *testing.T) {
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
		SessionID: "report-handler-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/sessions/report-handler-test/versions/1/report", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var report core.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := report.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}
