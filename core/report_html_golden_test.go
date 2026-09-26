package core_test

// PC-122's own acceptance criterion, verbatim: "HTML golden fixture byte-compared in
// CI, negative-controlled." Same pattern report_golden_test.go already established
// for the JSON report.

import (
	"os"
	"testing"

	"preflight/core"
	"preflight/render"
)

func buildGoldenReportForHTML(t *testing.T, sessionID string) core.Report {
	t.Helper()
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)
	findings := core.BuildFindings(ir, workload)
	scorecard := core.BuildScorecard(findings, 1)

	// The real, live-rendered diagram — exactly what cmd/gen-golden-fixtures used to
	// produce the committed fixture, so this test proves the SAME real SVG round-trips
	// byte-identically through RenderReportHTML, not a stand-in string.
	dot := core.RenderDOT(ir)
	svg, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("render diagram: %v", err)
	}

	return core.BuildReport(sessionID, 1, svg, ir, workload, findings, scorecard, nil, core.PriceTable{}, nil)
}

func TestGoldenReportHTML_ByteIdentical(t *testing.T) {
	report := buildGoldenReportForHTML(t, "golden")
	got, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}

	want, err := os.ReadFile("../golden/fixtures/aws.report.html")
	if err != nil {
		t.Fatalf("read golden fixture: %v (run `go run ./cmd/gen-golden-fixtures` if this report shape genuinely changed)", err)
	}

	if got != string(want) {
		t.Fatalf("golden/fixtures/aws.report.html is stale — regenerate with `go run ./cmd/gen-golden-fixtures` and hand-verify the diff before committing.\ngot %d bytes, want %d bytes", len(got), len(want))
	}
}

// TestGoldenReportHTML_ByteIdentical_NegativeControl mirrors report_golden_test.go's
// own negative control — proves the byte-comparison above actually discriminates.
func TestGoldenReportHTML_ByteIdentical_NegativeControl(t *testing.T) {
	report := buildGoldenReportForHTML(t, "deliberately-different-session-id")
	got, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}

	want, err := os.ReadFile("../golden/fixtures/aws.report.html")
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}

	if got == string(want) {
		t.Fatal("a report built with a different session_id byte-matched the golden HTML fixture — the byte-comparison test above cannot be trusted")
	}
}
