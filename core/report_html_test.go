package core_test

// PC-122's own acceptance criteria: "Test asserts mandatory sections/disclaimers are
// present in rendered output." Cost disclaimer, not_assessable reasons, compliance
// not-assessable-from-architecture counts, and the assumptions appendix — the Card's
// own explicit list of content a template must never drop.

import (
	"strings"
	"testing"

	"preflight/core"
)

func minimalReportForHTML() core.Report {
	ir := &core.IR{}
	workload := core.Workload{}
	report := core.BuildReport("s1", 1, "<svg>diagram</svg>", ir, workload, nil, core.Scorecard{VersionNumber: 1}, nil, core.PriceTable{}, nil)
	return report
}

func TestRenderReportHTML_ContainsMandatoryContent(t *testing.T) {
	report := minimalReportForHTML()
	html, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}

	mustContain := []string{
		"<svg>diagram</svg>",                  // the embedded PC-81 diagram, verbatim
		"Cost figures are estimates derived",  // the cost disclaimer, verbatim
		"Assumptions and provenance appendix", // the assumptions appendix section
		"not_assessable",                      // real not_assessable reasons appear
	}
	for _, s := range mustContain {
		if !strings.Contains(html, s) {
			t.Errorf("rendered HTML is missing mandatory content: %q", s)
		}
	}
}

func TestRenderReportHTML_GoldenBundle_ContainsComplianceNotAssessableCounts(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)
	findings := core.BuildFindings(ir, workload)
	scorecard := core.BuildScorecard(findings, 1)
	report := core.BuildReport("golden", 1, "<svg>diagram</svg>", ir, workload, findings, scorecard, nil, core.PriceTable{}, nil)

	html, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}

	// Hand-verified against the real golden PCI DSS catalog result (core/
	// compliance_catalog_test.go's own TestPCIDSS4_GoldenBundle_HandVerified):
	// PCI DSS shows 10 not-assessable-from-architecture controls.
	if !strings.Contains(html, "<td>pci_dss_4</td><td>2</td><td>0</td><td class=\"not-assessable\">10</td>") {
		t.Error("rendered HTML does not show the real golden PCI DSS not-assessable count (10)")
	}
	if !strings.Contains(html, report.FailureModes.Findings[0].ID) {
		t.Error("rendered HTML does not contain a real finding ID from the golden bundle")
	}
}

// TestRenderReportHTML_Deterministic proves the same Report value renders to
// byte-identical HTML every time — NFR-1, the same guarantee core.RenderDOT's own
// tests already prove for the diagram.
func TestRenderReportHTML_Deterministic(t *testing.T) {
	report := minimalReportForHTML()
	first, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := core.RenderReportHTML(report)
		if err != nil {
			t.Fatalf("RenderReportHTML: %v", err)
		}
		if again != first {
			t.Fatalf("run %d produced different output than the first run", i)
		}
	}
}

// TestRenderReportHTML_EscapesArchitectSuppliedText proves html/template's own
// escaping actually applies to real report content (never assumed) — an XSS-shaped
// finding title must never appear unescaped in the output.
func TestRenderReportHTML_EscapesArchitectSuppliedText(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	findings := []core.Finding{{
		ID: "finding.x", Title: "<script>alert(1)</script>",
		Dimensions: core.FailureMode{
			Trigger: "t", AffectedComponents: []string{"n1"}, Detection: core.DetectionModeled,
			Impact: core.NotAssessable[any]("x", prov).ToEnvelope(), Likelihood: core.DeriveLikelihood(prov).ToEnvelope(),
			Detectability: core.DeriveDetectability(core.DetectionModeled, prov).ToEnvelope(),
			Recoverability: core.Recoverability{
				FailoverPathExists: core.NotAssessable[any]("x", prov).ToEnvelope(),
				RPOFeasible:        core.NotAssessable[any]("x", prov).ToEnvelope(),
			},
		},
		Evidence: []core.EvidenceRef{{Description: "x"}},
		Outcome:  core.NotAssessable[any]("x", prov).ToEnvelope(),
	}}
	scorecard := core.BuildScorecard(findings, 1)
	report := core.BuildReport("s1", 1, "<svg>diagram</svg>", &core.IR{}, core.Workload{}, findings, scorecard, nil, core.PriceTable{}, nil)

	html, err := core.RenderReportHTML(report)
	if err != nil {
		t.Fatalf("RenderReportHTML: %v", err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("finding title was not escaped — real XSS risk in a rendered report")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("expected the finding title to appear HTML-escaped")
	}
	// The diagram itself must still render unescaped — it is real SVG, not text.
	if !strings.Contains(html, "<svg>diagram</svg>") {
		t.Error("the diagram SVG must render unescaped even though other content is escaped")
	}
}
