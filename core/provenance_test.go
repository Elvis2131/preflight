package core_test

import (
	"testing"

	"preflight/core"
)

// TestTaggedRequiresProvenance is PC-6's second acceptance criterion: "a test exists
// that attempts to construct a Tagged value without a provenance field and fails to do
// so". core.Tagged's fields are unexported, so this package (an external test package —
// note the `core_test` package name) has exactly one way to produce a Tagged[T]:
// core.NewTagged. This test proves that path itself refuses a missing provenance,
// rather than silently accepting a zero value.
func TestTaggedRequiresProvenance(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected NewTagged to panic on zero-value Provenance, but it returned normally")
		}
	}()

	var zeroProvenance core.Provenance
	core.NewTagged(42, zeroProvenance) // must panic — this is the "fails to do so"
}

// TestTaggedWithProvenanceSucceeds is the positive control: the same construction with
// real provenance must work, so the panic above is proven to be about the *missing*
// provenance specifically, not about NewTagged being broken outright.
func TestTaggedWithProvenanceSucceeds(t *testing.T) {
	prov := core.NewProvenance(core.KindStated, "workload.yaml:availability.rto_seconds")
	tagged := core.NewTagged(60, prov)

	if got := tagged.Value(); got != 60 {
		t.Errorf("Value() = %d, want 60", got)
	}
	if got := tagged.Provenance().Kind; got != core.KindStated {
		t.Errorf("Provenance().Kind = %q, want %q", got, core.KindStated)
	}
}

func TestNewProvenanceRejectsEmptySource(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected NewProvenance to panic on empty source")
		}
	}()
	core.NewProvenance(core.KindStated, "")
}
