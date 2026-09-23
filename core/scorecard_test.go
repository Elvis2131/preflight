package core_test

// This file is PC-19's second acceptance criterion, verified structurally via
// reflection — the same technique already established for Assessment[T]'s "no pass/
// fail field" and FailureMode's "no severity number" guarantees.

import (
	"reflect"
	"testing"

	"preflight/core"
)

func TestScorecardHasNoCompositeScoreField(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(core.Scorecard{}),
		reflect.TypeOf(core.ScorecardEntry{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			switch f.Type.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
				if f.Name == "VersionNumber" {
					continue // a version NUMBER identifies a version; it is not a composite score of anything
				}
				t.Errorf("%s has a numeric field %q (%s) — this is exactly the composite architecture score PC-19 forbids", typ.Name(), f.Name, f.Type.Kind())
			}
		}
	}
}

// TestDeltaEntryCarriesProvenance is PC-19's third acceptance criterion, verbatim:
// "every delta entry carries its own provenance tag." Checked both structurally (the
// field exists and is required) and behaviorally (an entry built by ComputeDelta
// actually has it populated).
func TestDeltaEntryCarriesProvenance(t *testing.T) {
	typ := reflect.TypeOf(core.DeltaEntry{})
	if _, ok := typ.FieldByName("Provenance"); !ok {
		t.Fatal("DeltaEntry has no Provenance field")
	}

	prov := core.NewProvenance(core.KindDerived, "test")
	entries := core.ComputeDelta(map[string]string{"a": "unsatisfied"}, map[string]string{"a": "satisfied"}, prov)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Provenance.IsZero() {
		t.Fatal("DeltaEntry.Provenance is zero-valued — every entry must carry a real provenance tag")
	}
	if err := entries[0].Validate(); err != nil {
		t.Fatalf("DeltaEntry failed its own schema validation: %v", err)
	}
}
