package render_test

// PC-122's own PDF half. Unlike render_test.go's own SVG tests (which fail hard —
// Graphviz is an already-established, CI-installed dependency this whole project
// assumes), these tests SKIP gracefully when wkhtmltopdf is absent: it is a brand
// new dependency introduced by this ticket, not yet verified installed/working
// anywhere (including this development environment) — failing hard here would make
// the whole suite red in every environment that hasn't installed it yet, which is
// exactly the "less trustworthy than it should be" outcome CLAUDE.md's own honesty
// standard warns against. See render/pdf.go's own doc comment for the PDF
// determinism decision (not byte-fixture-tested, unlike the HTML report).

import (
	"os/exec"
	"testing"

	"preflight/render"
)

func skipIfNoWkhtmltopdf(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("wkhtmltopdf"); err != nil {
		t.Skip("wkhtmltopdf not installed on PATH — skipping (see render/pdf.go's own doc comment; CI installs it, see .github/workflows/ci.yml)")
	}
}

func TestPDF_SmallHTML_ProducesRealPDF(t *testing.T) {
	skipIfNoWkhtmltopdf(t)
	pdf, err := render.PDF(`<html><body><h1>hello</h1></body></html>`)
	if err != nil {
		t.Fatalf("PDF: %v", err)
	}
	if len(pdf) < 4 || string(pdf[:4]) != "%PDF" {
		t.Errorf("output does not start with the real PDF magic bytes (%%PDF): got %q", pdf[:min(4, len(pdf))])
	}
}

func TestPDFRendererVersion_ReturnsRealVersionString(t *testing.T) {
	skipIfNoWkhtmltopdf(t)
	v, err := render.PDFRendererVersion()
	if err != nil {
		t.Fatalf("PDFRendererVersion: %v", err)
	}
	if v == "" {
		t.Error("got an empty version string, want a real one")
	}
}

func TestPDF_MissingBinary_ReturnsRealError(t *testing.T) {
	// This test does NOT skip — it specifically proves the "tool not found" path
	// (render.go's own SVG has the identical guarantee for a missing dot), which is
	// exactly the path this development environment (no wkhtmltopdf installed)
	// actually exercises today. If wkhtmltopdf IS installed here later, this test
	// naturally becomes untestable via LookPath and is skipped instead — never a
	// false failure either way.
	if _, err := exec.LookPath("wkhtmltopdf"); err == nil {
		t.Skip("wkhtmltopdf IS installed — this test only proves the not-found path")
	}
	_, err := render.PDF(`<html></html>`)
	if err == nil {
		t.Fatal("got nil error with wkhtmltopdf absent, want a real, specific error")
	}
}
