package analyse

import (
	"strings"
	"testing"
)

// This file tests AssessResolvable directly against its own minimal ResolvableElement
// shape — core-independent, per this package's now-enforced no-core-import rule (see
// resolution.go's doc comment). The ORIGINAL PC-11 acceptance-criteria tests, written
// against real core.Node/core.Edge values, now live in core/resolution_test.go, which
// exercises core.AssessNode/core.AssessEdge — the public wrappers around exactly the
// function tested here.

func TestAssessResolvable_Known(t *testing.T) {
	el := ResolvableElement{ID: "rds.payments", Unresolved: false}
	value, ok, _ := AssessResolvable(el, func() bool { return true })
	if !ok {
		t.Fatal("known element: expected ok=true, got not_assessable")
	}
	if !value {
		t.Error("known element: compute function's result was not propagated")
	}
}

func TestAssessResolvable_InferredTreatedAsKnown(t *testing.T) {
	// PRD §4: inferred has a real, derived value — the caller sets Unresolved=false
	// for both known and inferred (only core.ResolutionUnresolved maps to true); this
	// test documents that AssessResolvable itself has no separate "inferred" case,
	// consistent with PC-11's own decision (flat 3 resolution states, no sub-states).
	el := ResolvableElement{ID: "rds.payments", Unresolved: false}
	value, ok, _ := AssessResolvable(el, func() bool { return true })
	if !ok || !value {
		t.Fatal("inferred (Unresolved=false) element: expected ok=true, value=true")
	}
}

func TestAssessResolvable_Unresolved(t *testing.T) {
	el := ResolvableElement{
		ID:               "rds.payments",
		Unresolved:       true,
		ProvenanceReason: "references module.payments_db.instance_id, but module \"payments_db\" is not present in the bundle",
	}
	computeCalled := false
	_, ok, reason := AssessResolvable(el, func() bool {
		computeCalled = true
		return true // if this were ever read, it would be a false "pass" — see below
	})

	if computeCalled {
		t.Fatal("compute was called for an unresolved element — AssessResolvable must short-circuit before running the caller's logic at all")
	}
	if ok {
		t.Fatal("unresolved element: expected ok=false")
	}
	if reason == "" {
		t.Fatal("unresolved element: reason must be non-empty (I4)")
	}
	if !strings.Contains(reason, "module") {
		t.Errorf("reason should surface the element's own ProvenanceReason, got: %q", reason)
	}
}

func TestAssessResolvable_Unresolved_NoReasonGiven_UsesFallback(t *testing.T) {
	el := ResolvableElement{ID: "rds.payments", Unresolved: true} // ProvenanceReason left empty
	_, ok, reason := AssessResolvable(el, func() bool { return true })
	if ok {
		t.Fatal("expected ok=false")
	}
	if reason == "" {
		t.Fatal("expected a non-empty fallback reason even when ProvenanceReason wasn't set")
	}
}

// TestUnresolvedNeverProducesPassFail is PC-11's second, most important acceptance
// criterion, made concrete with a toy stand-in for a real analysis (the real
// redundancy/SPOF check is PC-14's job) — proving the STRUCTURAL guarantee: the
// compute callback never runs at all for an unresolved element, so it is
// architecturally incapable of returning a false "no redundancy" pass/fail.
func TestUnresolvedNeverProducesPassFail(t *testing.T) {
	hasRedundancy := func() bool {
		// A deliberately naive stand-in for PC-14's real redundancy check. If this
		// were ever invoked on an element with no real data, it would return the
		// zero value (false) — exactly the "no redundancy" false negative PC-11's
		// Card warns about.
		return false
	}

	unresolved := ResolvableElement{ID: "rds.payments", Unresolved: true, ProvenanceReason: "test"}
	_, ok, _ := AssessResolvable(unresolved, hasRedundancy)

	if ok {
		t.Fatal("an unresolved element must never resolve to ok==true — that is the pass/fail path this ticket exists to close off")
	}
}
