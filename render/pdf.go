// This file is PC-122's PDF half — the ONE place this codebase shells out to headless
// Chromium, mirroring render.go's own Graphviz `dot` boundary exactly: real subprocess
// I/O, isolated outside core/ (I1).
//
// WHY CHROMIUM, NOT wkhtmltopdf (PC-122's decision comment): wkhtmltopdf's repository was
// archived on 2 January 2023 and its last release was 10 June 2020 — an unmaintained binary
// is the wrong trade for a project whose claim is evidenced correctness. Chromium is
// maintained, is already in the toolchain via Playwright, and prints the SAME HTML the
// HTML export produces, so the PDF is a print of the golden-fixtured HTML, not a second
// layout engine with its own opinions.
//
// PDF DETERMINISM DECISION, recorded here as the Card's decision comment requires:
//
//   - The HTML report (core.RenderReportHTML) is the byte-stable, golden-fixtured artifact
//     (golden/fixtures/aws.report.html, core/report_html_golden_test.go).
//   - Chromium embeds a CreationDate and ModDate. They are the ONLY bytes that differed
//     between two renders of the same HTML in the same environment (measured: 4 bytes, the
//     seconds digits) and they have a fixed width, so normalizeDates replaces them with a
//     constant of the same length — the xref offsets stay valid, and two renders in one
//     environment are byte-identical (render/pdf_test.go proves it).
//   - There is deliberately NO golden PDF fixture. Even with the dates fixed, PDF bytes
//     depend on the exact Chromium build (the /Producer string names it) and on which
//     fonts the machine has to subset and embed, so a checked-in binary would only be
//     true on the machine that made it. Claiming byte-stability across machines or
//     Chromium versions would be the same overclaim render.go's Graphviz note already
//     refuses to make. The Chromium version is pinned in CI and logged, like `dot -V`.
package render

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// chromiumEnv names the environment variable that pins the exact Chromium binary to use
// (CI sets it so the logged version is the version that rendered). Opt-in sandbox
// disabling lives in chromiumNoSandboxEnv: a sandbox-less Chromium is only acceptable for
// rendering this server's own escaped HTML inside a throwaway container, so it is never
// the default.
const (
	chromiumEnv          = "PREFLIGHT_CHROMIUM"
	chromiumNoSandboxEnv = "PREFLIGHT_CHROMIUM_NO_SANDBOX"
	pdfTimeout           = 60 * time.Second
)

// fixedPDFDate is what every embedded creation/modification date is rewritten to. Same
// byte length as Chromium's own format, so no offset in the file moves.
const fixedPDFDate = "D:20000101000000+00'00'"

var pdfDateRE = regexp.MustCompile(`/(CreationDate|ModDate) \(D:\d{14}[+\-Z]\d{2}'\d{2}'\)`)

// normalizeDates rewrites Chromium's embedded /CreationDate and /ModDate to fixedPDFDate.
// Same-length by construction (the pattern matches the full fixed-width date).
func normalizeDates(pdf []byte) []byte {
	return pdfDateRE.ReplaceAll(pdf, []byte(`/$1 (`+fixedPDFDate+`)`))
}

// chromiumCandidates are searched, in order, when PREFLIGHT_CHROMIUM is not set: common
// names on PATH, then Playwright's own browser cache (where `npx playwright install
// chromium` puts the headless shell this project already uses).
func chromiumPath() (string, error) {
	if p := os.Getenv(chromiumEnv); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("render: %s=%q does not exist: %w", chromiumEnv, p, err)
		}
		return p, nil
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "chrome-headless-shell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	home, _ := os.UserHomeDir()
	var roots []string
	if cache := os.Getenv("PLAYWRIGHT_BROWSERS_PATH"); cache != "" {
		roots = append(roots, cache)
	}
	switch runtime.GOOS {
	case "darwin":
		roots = append(roots, filepath.Join(home, "Library", "Caches", "ms-playwright"))
	default:
		roots = append(roots, filepath.Join(home, ".cache", "ms-playwright"))
	}
	for _, root := range roots {
		matches, _ := filepath.Glob(filepath.Join(root, "chromium_headless_shell-*", "*", "chrome-headless-shell"))
		sort.Strings(matches)
		if len(matches) > 0 {
			return matches[len(matches)-1], nil // highest build number
		}
	}
	return "", fmt.Errorf("render: no headless Chromium found — set %s, put chromium on PATH, or run `npx playwright install chromium`", chromiumEnv)
}

// PDF prints html to a PDF with headless Chromium and returns the bytes with the embedded
// dates normalized. A missing Chromium, or any invocation failure, returns a real, specific
// error naming what happened — never a silently-empty or fabricated PDF standing in for a
// real one, the same discipline SVG (render.go) established for a missing Graphviz install.
func PDF(html string) ([]byte, error) {
	bin, err := chromiumPath()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "preflight-pdf-")
	if err != nil {
		return nil, fmt.Errorf("render: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "report.html")
	out := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(in, []byte(html), 0o600); err != nil {
		return nil, fmt.Errorf("render: write html: %w", err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: in}).String()

	args := []string{"--headless", "--disable-gpu", "--no-pdf-header-footer", "--print-to-pdf=" + out}
	if os.Getenv(chromiumNoSandboxEnv) == "1" {
		args = append(args, "--no-sandbox")
	}
	args = append(args, fileURL)

	ctx, cancel := context.WithTimeout(context.Background(), pdfTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render: chromium failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	pdf, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("render: chromium produced no PDF: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, fmt.Errorf("render: chromium output is not a PDF (%d bytes)", len(pdf))
	}
	return normalizeDates(pdf), nil
}

// PDFRendererVersion runs `chromium --version` and returns its own reported version
// string — used by .github/workflows/ci.yml to print (never silently assume) which exact
// Chromium build produced a CI run's PDF output, the same "dot -V" logging render.go's own
// CI step already does for Graphviz.
func PDFRendererVersion() (string, error) {
	bin, err := chromiumPath()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("render: chromium --version failed: %w", err)
	}
	return string(bytes.TrimSpace(out)), nil
}
