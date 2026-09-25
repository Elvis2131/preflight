package creds_test

// This file is PC-10's remaining acceptance criterion, made real: "P1 has zero imports
// of credential-handling code, verified by Go's internal/ package placement
// (compiler-enforced)." See creds.go's own doc comment for the full context — this is
// the direct Go-boundary counterpart to core/boundary_test.go's
// TestInternalBoundaryRejectsOutsideImport, same technique, applied to the P1/P2/P3
// process boundary (ADR-003) instead of core's own I1 boundary.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunnerdInternalBoundaryRejectsOutsideImport proves — by actually invoking the Go
// compiler, not by asserting a rule in a comment — that a package outside cmd/runnerd/
// cannot import cmd/runnerd/internal/creds. Placed as a sibling of cmd/assessd and
// cmd/reasond (the module root), exactly the position those two real P1/P2 binaries are
// already in: if this synthesized probe is rejected, so would a real import from either
// of them.
func TestRunnerdInternalBoundaryRejectsOutsideImport(t *testing.T) {
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	// creds/ -> internal/ -> runnerd/ -> cmd/ -> module root
	moduleRoot = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(moduleRoot))))

	scratchDir := filepath.Join(moduleRoot, "zz_p3_boundary_probe")
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(scratchDir) })

	src := `package p3boundaryprobe

import _ "preflight/cmd/runnerd/internal/creds"
`
	if err := os.WriteFile(filepath.Join(scratchDir, "probe.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command("go", "build", "./zz_p3_boundary_probe/...")
	cmd.Dir = moduleRoot
	out, buildErr := cmd.CombinedOutput()

	if buildErr == nil {
		t.Fatalf("expected build failure importing cmd/runnerd/internal/creds from outside cmd/runnerd/, but build succeeded:\n%s", out)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "internal") || !strings.Contains(outStr, "not allowed") {
		t.Fatalf("build failed, but not with the expected internal-import error; got:\n%s", out)
	}
}

// cloudSDKModulePrefixes are real, published Go module paths for the major cloud
// providers' SDKs — the actual "credential-handling code" this criterion means, not
// preflight's own providers/aws or providers/azure packages (Terraform resource-type
// mapping only, verified elsewhere this project never touches a live cloud API or
// credential — see providers/aws's and providers/azure's own package docs).
var cloudSDKModulePrefixes = []string{
	"github.com/aws/aws-sdk-go",
	"github.com/Azure/azure-sdk-for-go",
	"github.com/Azure/azure-sdk-for-go-extensions",
	"cloud.google.com/go",
	"google.golang.org/api",
	"github.com/hashicorp/aws-sdk-go-base",
}

// TestP1AndP2HaveZeroCloudSDKOrValidateImports is this criterion's other half: a real,
// live, CI-enforced fact about the CURRENT dependency graph (via `go list -deps`,
// invoking the real toolchain, not grepping source for the string "aws" — which would
// false-positive on providers/aws's own legitimate Terraform-mapping package name), not
// merely "nothing is violated because nothing credentialed has been built yet" (PC-10's
// own prior, honest, but static comment). This test is the active guard: it fails the
// moment a future change adds a cloud SDK import, or a direct dependency on
// preflight/validate (P3's own designated home, cmd/runnerd/main.go's own comment), to
// either P1 (cmd/assessd) or P2 (cmd/reasond).
func TestP1AndP2HaveZeroCloudSDKOrValidateImports(t *testing.T) {
	moduleRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	moduleRoot = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(moduleRoot))))

	for _, pkg := range []string{"./cmd/assessd/...", "./cmd/reasond/..."} {
		cmd := exec.Command("go", "list", "-deps", pkg)
		cmd.Dir = moduleRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		deps := strings.Fields(string(out))

		for _, dep := range deps {
			for _, prefix := range cloudSDKModulePrefixes {
				if strings.HasPrefix(dep, prefix) {
					t.Errorf("%s transitively imports %s, a real cloud SDK — this is exactly the credential-handling boundary ADR-003/PC-10 requires stay P3-only", pkg, dep)
				}
			}
			if dep == "preflight/validate" || strings.HasPrefix(dep, "preflight/validate/") {
				t.Errorf("%s transitively imports %s — validate/ is P3's own designated home (cmd/runnerd/main.go's own comment); P1/P2 must never depend on it", pkg, dep)
			}
			// PC-116/ADR-006: the real AWS Bulk Price List API fetcher is P3-only —
			// this is already structurally impossible (Go's own internal/ rule), but
			// checked here too for the same "live, CI-enforced fact" reason as the
			// checks above, not merely relying on the compiler error being noticed.
			if dep == "preflight/cmd/runnerd/internal/pricingfetch" {
				t.Errorf("%s transitively imports %s — the pricing fetcher is P3-only (ADR-006 §2); P1/P2 must never depend on it", pkg, dep)
			}
		}
	}
}
