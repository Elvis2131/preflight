package core_test

import (
	"reflect"
	"testing"

	"preflight/core"
)

// TestAssessmentHasNoPassFailField is PC-6's third acceptance criterion: "a test exists
// that attempts to return a pass/fail result without going through not_assessable and
// fails". Assessment[T]'s constructors (Assessed, NotAssessable) are the only exported
// way to produce one, and there is no bool-returning accessor that could stand in for
// a bare pass/fail — Value() and Reason() both use the comma-ok idiom precisely so a
// caller cannot get a T (or a reason) without also learning whether this assessment
// actually resolved.
//
// This test verifies that structurally, via reflection, rather than merely by
// inspecting the source by eye: it fails if anyone ever adds an exported bool field or
// a bare-bool-returning method to Assessment[T] in the future — the exact regression
// I4 exists to prevent.
func TestAssessmentHasNoPassFailField(t *testing.T) {
	typ := reflect.TypeOf(core.Assessment[int]{})

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath == "" { // PkgPath is empty only for exported fields
			t.Fatalf("Assessment[T] has an exported field %q — I4 requires zero exported fields (all state must flow through Assessed/NotAssessable)", f.Name)
		}
		if f.Type.Kind() == reflect.Bool {
			t.Fatalf("Assessment[T] has a bool-typed field %q — this is exactly the pass/fail collapse I4 forbids, even if unexported today", f.Name)
		}
	}

	assessmentType := reflect.TypeOf((*core.Assessment[int])(nil)).Elem()
	for i := 0; i < assessmentType.NumMethod(); i++ {
		m := assessmentType.Method(i)
		if m.Type.NumOut() == 1 && m.Type.Out(0).Kind() == reflect.Bool && m.Name != "IsAssessed" {
			t.Fatalf("Assessment[T] has method %s() returning a bare bool — only IsAssessed() may report a boolean, and it reports resolution state, not a verdict", m.Name)
		}
	}
}

func TestAssessedRequiresProvenance(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected Assessed to panic on zero-value Provenance")
		}
	}()
	var zero core.Provenance
	core.Assessed(true, zero)
}

func TestNotAssessableRequiresReasonAndProvenance(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "core/analyse:spof-detector")

	t.Run("empty reason panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected NotAssessable to panic on empty reason")
			}
		}()
		core.NotAssessable[bool]("", prov)
	})

	t.Run("zero provenance panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected NotAssessable to panic on zero Provenance")
			}
		}()
		var zero core.Provenance
		core.NotAssessable[bool]("dynamic block not in v1 parser subset", zero)
	})
}

// TestAssessmentCommaOkIdiom is the positive control proving the type actually works
// for its intended purpose, not just that it refuses misuse.
func TestAssessmentCommaOkIdiom(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "core/analyse:spof-detector")

	ok := core.Assessed(true, prov)
	if v, resolved := ok.Value(); !resolved || v != true {
		t.Errorf("Assessed(true).Value() = (%v, %v), want (true, true)", v, resolved)
	}
	if _, hasReason := ok.Reason(); hasReason {
		t.Error("a resolved Assessment must not report a Reason")
	}

	unknown := core.NotAssessable[bool]("route table not resolvable: dynamic block", prov)
	if v, resolved := unknown.Value(); resolved {
		t.Errorf("NotAssessable.Value() reported resolved=true with value %v — I4 violation", v)
	}
	if reason, hasReason := unknown.Reason(); !hasReason || reason == "" {
		t.Error("a NotAssessable Assessment must report a non-empty Reason")
	}
}
