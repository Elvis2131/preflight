package render_test

// PC-81: hand-verified against a small, real dot invocation before trusting a golden
// bundle's full round trip (render_golden_test.go).

import (
	"strings"
	"testing"

	"preflight/render"
)

func TestSVG_SmallGraph_ProducesRealSVG(t *testing.T) {
	dot := `digraph g { a -> b; }`
	svg, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("SVG: %v (is graphviz installed? \"dot -V\")", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Errorf("output does not contain an <svg> tag:\n%s", svg)
	}
	if !strings.HasPrefix(strings.TrimSpace(svg), "<?xml") {
		t.Errorf("output does not start with an XML declaration:\n%s", svg)
	}
}

func TestSVG_Deterministic_SameInputTwiceProducesByteIdenticalOutput(t *testing.T) {
	dot := `digraph g { a [label="Load Balancer"]; b [label="Database"]; a -> b [label="routes_to"]; }`
	first, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("SVG (first call): %v", err)
	}
	second, err := render.SVG(dot)
	if err != nil {
		t.Fatalf("SVG (second call): %v", err)
	}
	if first != second {
		t.Fatalf("dot -Tsvg produced different output for identical input across two separate invocations")
	}
}

func TestSVG_InvalidDOT_ReturnsRealError(t *testing.T) {
	_, err := render.SVG("this is not valid dot syntax {{{")
	if err == nil {
		t.Fatal("expected a real error for invalid DOT syntax, got nil")
	}
}
