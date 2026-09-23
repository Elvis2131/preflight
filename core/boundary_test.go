package core_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInternalBoundaryRejectsOutsideImport proves — by actually invoking the Go
// compiler, not by asserting a rule in a comment — that PC-6's first acceptance
// criterion holds: a package outside core/ cannot import core/internal/ir (or its
// siblings). This is I1's structural half: ingest/, reason/, server/ and providers/ are
// physically unable to reach core/internal/* except through the pure API core/ itself
// exposes.
//
// The technique: synthesize a throwaway package inside this module (Go's internal/
// visibility is scoped by module + directory position, not by an external test
// runner's location) that imports core/internal/ir, and confirm `go build` rejects it
// with the internal-package error. A package outside core/ cannot legally exist inside
// core/'s own tree for this purpose, so the scratch package is placed as a sibling of
// core/ (module root), which is exactly the position ingest/, reason/, server/ and
// providers/ are already in.
func TestInternalBoundaryRejectsOutsideImport(t *testing.T) {
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	moduleRoot = filepath.Dir(moduleRoot) // core/ -> module root

	scratchDir := filepath.Join(moduleRoot, "zz_boundary_probe")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(scratchDir) })

	src := `package boundaryprobe

import _ "preflight/core/internal/ir"
`
	if err := os.WriteFile(filepath.Join(scratchDir, "probe.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "build", "./zz_boundary_probe/...")
	cmd.Dir = moduleRoot
	out, buildErr := cmd.CombinedOutput()

	if buildErr == nil {
		t.Fatalf("expected build failure importing core/internal/ir from outside core/, but build succeeded:\n%s", out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "internal") || !strings.Contains(outStr, "not allowed") {
		t.Fatalf("build failed, but not with the expected internal-import error; got:\n%s", out)
	}
}
