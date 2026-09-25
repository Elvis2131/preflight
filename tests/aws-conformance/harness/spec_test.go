package harness

// PC-119's first acceptance criterion, hand-verified directly against the harness
// itself (not just observed indirectly through a real conformance test): "a test
// missing its citation field fails lint/CI." validate is Verify's own logic (Verify
// is a thin t.Fatal wrapper around it) — testing it directly here, package-internal,
// avoids the nested-testing.T trickery a black-box test of Verify itself would need.

import "testing"

func completeSpec() Spec {
	return Spec{
		ID:            "TEST-001",
		Rule:          "some rule",
		Source:        "https://docs.aws.amazon.com/example",
		Scenario:      "some scenario",
		Configuration: "some config",
		Request:       "some request",
		Expected:      "some expectation",
	}
}

func TestValidate_CompleteSpec_Passes(t *testing.T) {
	if err := validate(completeSpec()); err != nil {
		t.Fatalf("a fully-populated spec was rejected: %v", err)
	}
}

func TestValidate_RejectsMissingEachRequiredField(t *testing.T) {
	cases := []struct {
		name       string
		break_func func(Spec) Spec
	}{
		{"ID", func(s Spec) Spec { s.ID = ""; return s }},
		{"Rule", func(s Spec) Spec { s.Rule = ""; return s }},
		{"Source", func(s Spec) Spec { s.Source = ""; return s }},
		{"Scenario", func(s Spec) Spec { s.Scenario = ""; return s }},
		{"Configuration", func(s Spec) Spec { s.Configuration = ""; return s }},
		{"Request", func(s Spec) Spec { s.Request = ""; return s }},
		{"Expected", func(s Spec) Spec { s.Expected = ""; return s }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := c.break_func(completeSpec())
			if err := validate(spec); err == nil {
				t.Fatalf("validate accepted a spec with an empty %s field", c.name)
			}
		})
	}
}

func TestValidate_RejectsNonURLSource(t *testing.T) {
	spec := completeSpec()
	spec.Source = "AWS RDS User Guide, page 42"
	if err := validate(spec); err == nil {
		t.Fatal("validate accepted a Source that isn't a URL")
	}
}
