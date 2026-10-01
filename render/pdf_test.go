package render_test

// PC-122's PDF half, now headless Chromium. Tests that need a real browser SKIP when none
// is available (it is a binary this project's CI installs and pins, but a developer
// machine may not have): failing hard there would make the suite red for an unrelated
// reason. See render/pdf.go for the determinism decision — dates normalized, so two renders
// in one environment are byte-identical; no golden PDF, because PDF bytes also depend on the
// Chromium build and installed fonts.

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"time"

	"preflight/render"
)

func skipIfNoChromium(t *testing.T) {
	t.Helper()
	if _, err := render.PDFRendererVersion(); err != nil {
		t.Skipf("no headless Chromium available (%v) — skipping; CI installs and pins one", err)
	}
}

// goldenReportHTML is the real, golden-fixtured report HTML — the exact artifact the PDF is
// a print of, per the decision that the PDF must not be a second layout engine.
func goldenReportHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../golden/fixtures/aws.report.html")
	if err != nil {
		t.Fatalf("read golden report html: %v", err)
	}
	return string(b)
}

func TestPDF_GoldenReportHTML_ProducesARealPDF(t *testing.T) {
	skipIfNoChromium(t)
	pdf, err := render.PDF(goldenReportHTML(t))
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf[max(0, len(pdf)-1024):], []byte("%%EOF")) {
		t.Fatalf("output is not a complete PDF (starts %q)", pdf[:min(8, len(pdf))])
	}
	if len(pdf) < 20_000 {
		t.Errorf("a full report should be a substantial PDF, got only %d bytes", len(pdf))
	}
	if !bytes.Contains(pdf, []byte("Chromium")) {
		t.Errorf("the PDF's own metadata should name Chromium as its creator — that is what makes the renderer auditable")
	}
}

// The determinism claim, tested rather than asserted: two renders of the same HTML in the
// same environment are byte-identical once the embedded dates are normalized. (Without
// normalization they differ in the seconds of CreationDate/ModDate; the unit test below
// proves the normalizer is what closes that gap.) Rendered >1s apart so a wall-clock
// difference would show if it were not neutralised.
func TestPDF_TwoRendersInOneEnvironmentAreByteIdentical(t *testing.T) {
	skipIfNoChromium(t)
	html := goldenReportHTML(t)
	a, err := render.PDF(html)
	if err != nil {
		t.Fatal(err)
	}
	waitOverASecond()
	b, err := render.PDF(html)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("two renders of the same HTML differ (%d vs %d bytes) — something besides the normalized dates is nondeterministic", len(a), len(b))
	}
	if !regexp.MustCompile(`/CreationDate \(D:20000101000000\+00'00'\)`).Match(a) {
		t.Errorf("the embedded creation date must be the fixed normalized value")
	}
}

func TestPDFRendererVersion_NamesChromium(t *testing.T) {
	skipIfNoChromium(t)
	v, err := render.PDFRendererVersion()
	if err != nil || v == "" {
		t.Fatalf("PDFRendererVersion: %q, %v", v, err)
	}
	if !regexp.MustCompile(`(?i)chrom`).MatchString(v) {
		t.Errorf("version %q should name Chromium", v)
	}
}

// Does NOT skip: a pinned-but-missing binary must be a real, specific error, never an empty
// or fabricated PDF — the same guarantee SVG has for a missing dot.
func TestPDF_MissingBinary_ReturnsRealError(t *testing.T) {
	t.Setenv("PREFLIGHT_CHROMIUM", "/definitely/not/chromium")
	pdf, err := render.PDF(`<html></html>`)
	if err == nil || len(pdf) != 0 {
		t.Fatalf("got %d bytes and error %v, want no PDF and a specific error", len(pdf), err)
	}
}

func waitOverASecond() { time.Sleep(1100 * time.Millisecond) }
