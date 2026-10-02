package server_test

// PC-154: ADR-003's process boundary, made a test. P1 (the assessment engine) must have NO path
// to reason/ (the LLM layer, P2) and no way to reach the API key — not by import, not by name.
// P2 is reached over HTTP by URL only; the wire types it shares with P1 live in core (data only).

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestP1_HasNoImportPathToTheLLMLayer(t *testing.T) {
	root := repoRoot(t)
	for _, pkg := range []string{"./cmd/assessd", "./server", "./core", "./ingest"} {
		cmd := exec.Command("go", "list", "-deps", pkg)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if dep == "preflight/reason" || strings.HasPrefix(dep, "preflight/reason/") {
				t.Errorf("%s depends on %s — P1 must have no path to the LLM layer (ADR-003)", pkg, dep)
			}
		}
	}
}

// The key and the provider's address exist in P2 and its tooling only. A name P1 never mentions
// is a name P1 cannot read.
func TestP1_NeverMentionsTheAPIKeyOrTheProvider(t *testing.T) {
	root := repoRoot(t)
	forbidden := []string{"NVIDIA_API_KEY", "nvapi-", "integrate.api.nvidia.com"}
	for _, dir := range []string{"server", "core", "ingest", "providers", "render", "pricing", "cmd/assessd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			for _, bad := range forbidden {
				if strings.Contains(string(b), bad) {
					t.Errorf("%s mentions %q — only the reason worker (reason/, cmd/reasond) and its eval may", path, bad)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
