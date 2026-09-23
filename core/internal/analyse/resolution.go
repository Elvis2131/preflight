// Package analyse is SPOF/min-cut, reachability, capacity, and the compliance rule
// engine (CLAUDE.md §6). This file is its foundational primitive: the mechanism that
// makes PC-11's second acceptance criterion — "a node with an unresolved reference
// never produces a pass/fail finding — only not_assessable" — a property every future
// analysis function gets for free, rather than a rule each one has to remember to
// re-implement.
//
// This package has no dependency on preflight/core — core imports analyse, never the
// reverse (see core/assess.go's doc comment: "core is the only public surface onto
// core/internal/{ir,analyse,simulate}", which requires analyse itself to know nothing
// of core's own types, only of plain Go values). AssessResolvable below is the
// core-independent primitive; core.AssessNode/core.AssessEdge (core/assess.go) are the
// core.Node/core.Edge-shaped wrappers PC-11 originally delivered as this file's public
// API — same behavior, same tests' intent, relocated to where it can compile without a
// cycle.
package analyse

// ResolvableElement is the minimal shape AssessResolvable needs from either a
// core.Node or a core.Edge — just enough to apply I4's gate without this package
// knowing either of those types exist.
type ResolvableElement struct {
	ID               string
	Unresolved       bool
	ProvenanceReason string // the element's own Provenance.Reason, if any was recorded
}

// AssessResolvable is the only sanctioned way an analysis function reads a resolvable
// element's value. It checks Unresolved before compute ever runs: an unresolved
// element short-circuits straight to a not_assessable result, so a future analysis
// author cannot forget the check — they cannot reach the element's data at all without
// going through this gate.
//
// "inferred" is treated the same as "known" by the caller constructing ResolvableElement
// (Unresolved should be true ONLY for core.ResolutionUnresolved) deliberately: PRD §4
// defines inferred as "resolved from a reference the engine could follow" — the engine
// has an actual value, just one it derived rather than one stated directly. Only
// unresolved ("references a declaration not present in the bundle") means there is
// genuinely nothing to compute over.
//
// Returns (value, true, "") when compute ran, or (zero, false, reason) when the
// element was unresolved — core.AssessNode/core.AssessEdge turn the latter into a real
// core.Assessment[T]'s not_assessable state.
func AssessResolvable[T any](el ResolvableElement, compute func() T) (value T, ok bool, notAssessableReason string) {
	if el.Unresolved {
		var zero T
		reason := el.ProvenanceReason
		if reason == "" {
			reason = "references a declaration not present in the bundle (PRD §4)"
		}
		return zero, false, "element " + el.ID + " is unresolved: " + reason
	}
	return compute(), true, ""
}
