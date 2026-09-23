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
