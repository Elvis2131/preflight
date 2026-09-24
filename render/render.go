// Package render (PC-81) is the ONE place this codebase shells out to the Graphviz
// `dot` binary — real subprocess I/O, which is exactly why this lives outside core/
// (I1: core/ does no I/O of any kind, enforced both by the compiler (internal/
// package placement) and by .golangci.yml's depguard/forbidigo rules as
// defense-in-depth). core/graph.go's RenderDOT does the actual, pure, deterministic
// IR-to-DOT-text conversion; this package's only job is handing that text to a real
// `dot` process and returning what it prints back.
package render

import (
	"bytes"
	"fmt"
	"os/exec"
)

// SVG invokes `dot -Tsvg` with dot as its stdin and returns the SVG it prints to
// stdout. A missing Graphviz install, or any other invocation failure, returns a
// real, specific error naming what happened — never a silently-empty or fabricated
// SVG string standing in for a real one.
//
// Determinism scope, stated precisely rather than implied more broadly: verified
// (render_test.go, render_golden_test.go) that the SAME dot input produces
// byte-identical SVG output across repeated invocations of a GIVEN, FIXED Graphviz
// install (Homebrew's 16.1.0, locally; whatever `.github/workflows/ci.yml` installs
// via apt in CI — the workflow prints `dot -V` so that version is always visible,
// not silently assumed). This is NOT a claim that two DIFFERENT Graphviz versions
// produce identical output for the same input — Graphviz's own layout engine has
// changed between major versions historically, and no two actual different versions
// have been compared here. What IS guarded, not just disclosed: a version drift
// between two separate CI runs (an apt/brew upgrade on the runner) would previously
// have passed silently, since render_golden_test.go only ever compared two
// invocations within the same run against each other. render_fixture_test.go closes
// that gap — it byte-compares live output against a CHECKED-IN fixture
// (render/testdata/golden_aws.svg), so any drift, whatever its cause, fails CI
// immediately rather than sitting undetected. See render/testdata/README.md for what
// a failure there means and how to regenerate responsibly.
func SVG(dot string) (string, error) {
	if _, err := exec.LookPath("dot"); err != nil {
		return "", fmt.Errorf("render: graphviz's \"dot\" binary not found on PATH: %w", err)
	}

	cmd := exec.Command("dot", "-Tsvg")
	cmd.Stdin = bytes.NewBufferString(dot)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render: dot -Tsvg failed: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}
