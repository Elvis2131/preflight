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
// changed between major versions historically, and nothing here has tested that
// cross-version case. If assessd's SVG output is ever expected to be byte-identical
// across two different deployment environments, THAT requires pinning the exact
// same Graphviz version in both — a real, current gap (only a single version is
// verified against, not proven invariant across versions), not a silent assumption.
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
