// This file is PC-122's own PDF half — the ONE place this codebase shells out to
// wkhtmltopdf, mirroring render.go's own Graphviz `dot` boundary exactly: real
// subprocess I/O, isolated outside core/ (I1).
//
// PDF DETERMINISM DECISION, recorded here per the Card's own explicit instruction
// ("Decide and record it — the same honesty as the Graphviz version scope note in
// render.go"): the HTML report (core.RenderReportHTML) is golden-fixture tested and
// proven byte-identical for the same Report input (core/report_html_golden_test.go).
// The PDF conversion step is NOT claimed byte-stable and is NOT golden-fixture
// tested — wkhtmltopdf (like most HTML-to-PDF tools built on WebKit/Qt) embeds a
// CreationDate/ModDate in the PDF's own /Info dictionary by default, and font
// subsetting/embedding can vary by the exact font files installed on the rendering
// machine, neither of which this package attempts to strip or normalize. Verifying
// PDF content instead means: parsing the PDF back out (core/report_pdf_test.go, when
// the tool is available) and asserting real report text appears in the extracted
// content stream — a real, weaker-but-honest guarantee, not a byte-for-byte one.
package render

import (
	"bytes"
	"fmt"
	"os/exec"
)

// PDF invokes `wkhtmltopdf - -` (stdin -> stdout) with html as its input and returns
// the PDF bytes it prints. A missing wkhtmltopdf install, or any other invocation
// failure, returns a real, specific error naming what happened — never a
// silently-empty or fabricated PDF standing in for a real one, the same discipline
// SVG (render.go) already established for a missing Graphviz install.
func PDF(html string) ([]byte, error) {
	if _, err := exec.LookPath("wkhtmltopdf"); err != nil {
		return nil, fmt.Errorf("render: \"wkhtmltopdf\" binary not found on PATH: %w", err)
	}

	cmd := exec.Command("wkhtmltopdf", "--quiet", "-", "-")
	cmd.Stdin = bytes.NewBufferString(html)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render: wkhtmltopdf failed: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// PDFRendererVersion runs `wkhtmltopdf --version` and returns its own reported
// version string — used by .github/workflows/ci.yml to print (never silently
// assume) which exact renderer build produced a given CI run's PDF output, the same
// "dot -V" logging render.go's own CI step already does for Graphviz.
func PDFRendererVersion() (string, error) {
	if _, err := exec.LookPath("wkhtmltopdf"); err != nil {
		return "", fmt.Errorf("render: \"wkhtmltopdf\" binary not found on PATH: %w", err)
	}
	out, err := exec.Command("wkhtmltopdf", "--version").Output()
	if err != nil {
		return "", fmt.Errorf("render: wkhtmltopdf --version failed: %w", err)
	}
	return string(bytes.TrimSpace(out)), nil
}
