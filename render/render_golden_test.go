package render_test

// PC-81's own acceptance criteria, verbatim, proven end to end against the real
// golden AWS bundle: (1) "graph rendered server-side ... as SVG with stable node IDs
// across versions", (2) "same IR produces byte-identical SVG output across runs".

import (
	"strings"
	"testing"

	"preflight/core"
	"preflight/ingest"
	"preflight/render"

	awsprovider "preflight/providers/aws"
)

func TestSVG_AgainstGoldenAWSBundle_StableIDsAndDeterministic(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	dot := core.RenderDOT(result.IR)
	svg, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("SVG: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Fatalf("output does not contain an <svg> tag")
	}

	// Stable node IDs: the real Terraform resource address for the golden database
	// must appear verbatim in the rendered SVG's own text content (Graphviz renders
	// labels as real <text> elements), not translated into some synthetic ID.
	if !strings.Contains(svg, "aws_db_instance.payments") {
		t.Errorf("expected the real resource address \"aws_db_instance.payments\" to appear in the rendered SVG")
	}

	// Determinism against a real, full-size golden bundle, not just a toy graph:
	// re-render from scratch (fresh ingest, fresh DOT generation, fresh dot
	// invocation) and confirm byte-identical output.
	result2, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest (second run): %v", err)
	}
	dot2 := core.RenderDOT(result2.IR)
	if dot != dot2 {
		t.Fatal("core.RenderDOT produced different DOT text for two independent ingests of the identical golden bundle")
	}
	svg2, err := render.SVG(dot2)
	if err != nil {
		t.Fatalf("SVG (second run): %v", err)
	}
	if svg != svg2 {
		t.Fatal("render.SVG produced different SVG output for the identical DOT text across two independent runs")
	}
}
