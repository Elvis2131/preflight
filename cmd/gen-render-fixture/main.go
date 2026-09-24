// Command gen-render-fixture derives render/testdata/golden_aws.{dot,svg} from a real
// run of core.RenderDOT + render.SVG against golden/aws — the same "derive from a
// first correct run, then hand-verify" discipline cmd/gen-golden-fixtures already
// established for IR/findings (PC-15), extended here to PC-81's own render fixtures.
// These files are what render/render_fixture_test.go compares live output against —
// see render/testdata/README.md for what a failure there means and how to regenerate
// responsibly, not blindly.
//
// Run with: go run ./cmd/gen-render-fixture
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"preflight/core"
	"preflight/ingest"
	"preflight/render"

	awsprovider "preflight/providers/aws"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}

	reg, err := awsprovider.Load()
	if err != nil {
		fail(fmt.Errorf("load AWS provider mappings: %w", err))
	}
	result, err := ingest.Ingest(filepath.Join(root, "golden", "aws"), reg, 1)
	if err != nil {
		fail(fmt.Errorf("ingest golden/aws: %w", err))
	}

	dot := core.RenderDOT(result.IR)
	svg, err := render.SVG(dot)
	if err != nil {
		fail(fmt.Errorf("render.SVG: %w (is graphviz installed? \"dot -V\")", err))
	}

	outDir := filepath.Join(root, "render", "testdata")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "golden_aws.dot"), []byte(dot), 0o644); err != nil {
		fail(fmt.Errorf("write golden_aws.dot: %w", err))
	}
	if err := os.WriteFile(filepath.Join(outDir, "golden_aws.svg"), []byte(svg), 0o644); err != nil {
		fail(fmt.Errorf("write golden_aws.svg: %w", err))
	}

	fmt.Println("wrote render/testdata/golden_aws.dot and golden_aws.svg")
	fmt.Println("update render/testdata/README.md's own Graphviz version note before committing")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-render-fixture:", err)
	os.Exit(1)
}
