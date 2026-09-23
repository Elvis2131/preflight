package core_test

// This file is PC-17's first and second acceptance criteria, verified structurally —
// via reflection, the same technique core/assessment_test.go already established for
// I4's "no pass/fail field" guarantee — not merely by reading the source today and
// trusting nobody adds a violation later.

import (
	"reflect"
	"testing"

	"preflight/core"
)

// TestFailureModeHasNoCompositeSeverityField is PC-17's first acceptance criterion:
// "no code path can produce a single combined severity number." Checked structurally:
// FailureMode must have no exported numeric field anywhere (which would be exactly
// the composite score CLAUDE.md §10 forbids), and its four dimensions must remain
// four separate fields, never collapsed into one.
func TestFailureModeHasNoCompositeSeverityField(t *testing.T) {
	typ := reflect.TypeOf(core.FailureMode{})

	wantDimensionFields := map[string]bool{
		"Impact": false, "Likelihood": false, "Detectability": false, "Recoverability": false,
	}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			// AffectedComponents/BlastRadius are []string, not numeric, so this branch
			// firing at all on any field is itself the violation being guarded against.
			t.Fatalf("FailureMode has a numeric field %q (%s) — this is exactly the composite severity number PC-17 forbids", f.Name, f.Type.Kind())
		}
		if _, tracked := wantDimensionFields[f.Name]; tracked {
			wantDimensionFields[f.Name] = true
		}
	}
	for name, found := range wantDimensionFields {
		if !found {
			t.Errorf("FailureMode is missing expected dimension field %q — the four dimensions must remain separate, named fields", name)
		}
	}
}

// TestDetectionStateOnlyTakesFourDefinedValues is PC-17's second acceptance criterion,
// verbatim: "Detection field only ever takes one of the four defined values, never a
// boolean." Checked two ways: the Go type itself is a string (not bool — a compile-
// time fact this test documents), and the frozen schema/validator rejects any value
// outside the four.
func TestDetectionStateOnlyTakesFourDefinedValues(t *testing.T) {
	// Compile-time fact: DetectionState's underlying kind is string, not bool. If a
	// future edit changes this to `type DetectionState bool`, this line stops
	// compiling — a stronger guarantee than a runtime check could give.
	var _ string = string(core.DetectionModeled)

	valid := []core.DetectionState{
		core.DetectionModeled, core.DetectionDeclared, core.DetectionUnknown, core.DetectionObserved,
	}
	if len(valid) != 4 {
		t.Fatalf("expected exactly 4 defined DetectionState values, got %d", len(valid))
	}

	fm := validFailureMode()
	for _, v := range valid {
		fm.Detection = v
		if err := fm.Validate(); err != nil {
			t.Errorf("Detection=%q should be valid: %v", v, err)
		}
	}

	fm.Detection = "true" // the exact failure mode this criterion names: collapsing to a boolean-like string
	if err := fm.Validate(); err == nil {
		t.Fatal("expected validation error for Detection=\"true\" — a boolean-shaped value is not one of the four defined states")
	}

	fm.Detection = "garbage"
	if err := fm.Validate(); err == nil {
		t.Fatal("expected validation error for an undefined Detection value")
	}
}

func validFailureMode() core.FailureMode {
	prov := core.NewProvenance(core.KindDerived, "test")
	return core.FailureMode{
		Trigger:            "test trigger",
		AffectedComponents: []string{"node.a"},
		Detection:          core.DetectionModeled,
		Impact:             core.Assessed[any]("test", prov).ToEnvelope(),
		Likelihood:         core.NotAssessable[any]("test", prov).ToEnvelope(),
		Detectability:      core.Assessed[any]("test", prov).ToEnvelope(),
		Recoverability: core.Recoverability{
			FailoverPathExists: core.Assessed[any](true, prov).ToEnvelope(),
			RPOFeasible:        core.NotAssessable[any]("test", prov).ToEnvelope(),
		},
	}
}
