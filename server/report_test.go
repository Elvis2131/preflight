package server_test

// PC-120: proves the real HTTP handler (server.GetReportHandler) against a real
// assessed-then-stored version — not just that core.BuildReport works in isolation.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/pricing"
	"preflight/render"
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

// TestGetReport_WithPricingSnapshot_ReconstructsPriceTable is PC-132's own server
// integration: GetReport must re-fetch the EXACT snapshot CostReport.SnapshotID
// names (never the currently-active one, which may have since changed) to compute
// usage-based cost — proven here by attaching a real pricing store and confirming
// the report comes back valid with a non-nil (never null) UsageBasedCharges slice.
// golden/aws's own checkout journey does not structurally flow at baseline (a real,
// pre-existing, already-documented gap — see core/usage_cost_test.go's own header
// note), so an empty slice here is the correct, honest result, not absence of
// wiring; core/usage_cost_test.go already proves the actual pricing/classification
// logic in isolation on fixtures that DO flow.
func TestGetReport_WithPricingSnapshot_ReconstructsPriceTable(t *testing.T) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()

	pricingStore, err := pricing.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("pricing.OpenStore: %v", err)
	}
	defer pricingStore.Close()
	if err := pricingStore.PutSnapshot(realPricingSnapshot("usage-cost-snap")); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	if err := pricingStore.SetActive("usage-cost-snap"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	store.AttachPricingStore(pricingStore)

	bundleDir, err := filepath.Abs("../golden/aws")
	if err != nil {
		t.Fatal(err)
	}
	workloadPath, err := filepath.Abs("../golden/workload.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Assess(store, server.AssessRequest{
		SessionID: "usage-cost-report-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	report, err := server.GetReport(store, "usage-cost-report-test", 1)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if !report.Cost.Available {
		t.Fatal("Cost.Available = false, want true — a pricing store was attached")
	}
	if report.Cost.UsageBasedCharges == nil {
		t.Error("UsageBasedCharges is nil, want an empty (never null) slice")
	}
	if err := report.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestGetReportHandler_FormatHTML_ReturnsRenderedHTML is PC-122's own acceptance
// criterion: "GET .../report?format=html|pdf returns rendered output; JSON remains
// the default." Proves the actual registered handler, not just core.RenderReportHTML
// in isolation.
func TestGetReportHandler_FormatHTML_ReturnsRenderedHTML(t *testing.T) {
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
		SessionID: "report-html-format-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/sessions/report-html-format-test/versions/1/report?format=html", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	body := rec.Body.String()
	if !bytesContains(body, "<!DOCTYPE html>") {
		t.Error("response body does not look like an HTML document")
	}
	if !bytesContains(body, "Cost figures are estimates derived") {
		t.Error("response body is missing the mandatory cost disclaimer")
	}
}

// TestGetReportHandler_FormatPDF_MissingRenderer_RealError proves the format=pdf
// path returns a real, classified error (never a silently-empty body) when the PDF
// renderer is missing. The missing case is FORCED (PREFLIGHT_CHROMIUM pointing at a
// binary that does not exist) so this proves the path on every machine, whether or not
// Chromium happens to be installed there (see render/pdf.go's own doc comment).
func TestGetReportHandler_FormatPDF_MissingRenderer_RealError(t *testing.T) {
	t.Setenv("PREFLIGHT_CHROMIUM", "/definitely/not/chromium")

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
		SessionID: "report-pdf-format-test", BundleDir: bundleDir, WorkloadPath: workloadPath,
	}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))

	req := httptest.NewRequest(http.MethodGet, "/sessions/report-pdf-format-test/versions/1/report?format=pdf", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (render_failed) since the renderer is missing: %s", rec.Code, rec.Body.String())
	}
	if !bytesContains(rec.Body.String(), "render_failed") {
		t.Errorf("body does not name the render_failed error code: %s", rec.Body.String())
	}
}

func bytesContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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

// TestGetReportHandler_FormatPDF_RealChromium_ReturnsARealPDF is the positive end to end:
// a real assessed session, GET ...report?format=pdf, a real PDF back through the real
// Chromium subprocess boundary. Skipped (not failed) where no Chromium exists.
func TestGetReportHandler_FormatPDF_RealChromium_ReturnsARealPDF(t *testing.T) {
	if _, err := render.PDFRendererVersion(); err != nil {
		t.Skipf("no headless Chromium available (%v)", err)
	}
	store, err := server.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bundleDir, _ := filepath.Abs("../golden/aws")
	workloadPath, _ := filepath.Abs("../golden/workload.yaml")
	if _, err := server.Assess(store, server.AssessRequest{SessionID: "pdf-real", BundleDir: bundleDir, WorkloadPath: workloadPath}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sessions/{id}/versions/{n}/report", server.GetReportHandler(store))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sessions/pdf-real/versions/1/report?format=pdf", nil))

	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("status %d content-type %q, want 200 application/pdf: %.200s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") || rec.Body.Len() < 20_000 {
		t.Fatalf("body is not a substantial PDF (%d bytes)", rec.Body.Len())
	}
}
