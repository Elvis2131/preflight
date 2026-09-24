package render_test

// This test is the actual guard the prior "determinism verified" language was
// missing: render_golden_test.go only ever compared two invocations WITHIN the same
// test run, both against whatever Graphviz happened to be installed at that moment —
// it could never catch the runner's Graphviz version drifting between two SEPARATE
// CI runs (an apt/brew upgrade on the CI image, a different machine). This test
// compares live output against a CHECKED-IN fixture (render/testdata/golden_aws.{dot,svg}),
// so a version drift fails CI loudly and immediately, the moment it happens — not a
// `dot -V` log line someone would have to think to go check. Same discipline
// golden/fixtures/*.json already uses for IR/findings, extended to the one place this
// codebase's own determinism claim depends on a THIRD-PARTY binary's version, not just
// on this codebase's own code.

import (
	"os"
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/render"

	awsprovider "preflight/providers/aws"
)

func TestRenderDOT_MatchesCheckedInGoldenFixture(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	got := core.RenderDOT(result.IR)

	want, err := os.ReadFile("testdata/golden_aws.dot")
	if err != nil {
		t.Fatalf("read testdata/golden_aws.dot: %v", err)
	}
	if got != string(want) {
		t.Errorf("core.RenderDOT's output no longer matches testdata/golden_aws.dot — pure Go, no external dependency, so this means a REAL code change to RenderDOT itself (never a Graphviz version issue, since this never invokes dot). If the change is intentional, regenerate the fixture (see render/testdata/README.md) and review the diff before committing it.")
	}
}

// TestSVG_MatchesCheckedInGoldenFixture is the guard against Graphviz version drift
// specifically. A failure here means EITHER a real code change to RenderDOT/SVG's own
// logic, OR — just as real, and the actual gap this test exists to close — the
// installed Graphviz version producing different output than the version
// testdata/golden_aws.svg was generated against. Both are failures CI should surface
// immediately, not accept silently: see render/testdata/README.md for how to tell
// which happened and what to do about it.
func TestSVG_MatchesCheckedInGoldenFixture(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	dot := core.RenderDOT(result.IR)
	got, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("SVG: %v", err)
	}

	want, err := os.ReadFile("testdata/golden_aws.svg")
	if err != nil {
		t.Fatalf("read testdata/golden_aws.svg: %v", err)
	}
	if got != string(want) {
		t.Errorf("render.SVG's output no longer matches testdata/golden_aws.svg. This is the byte-for-byte determinism guarantee failing for real — either a code change, or (the gap this test exists to close) this environment's installed Graphviz version differs from the one testdata/golden_aws.svg was generated against. See render/testdata/README.md before regenerating.")
	}
}
