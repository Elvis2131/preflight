// This file is PC-14's second acceptance criterion, and per its own Conversation, the
// rule "most likely to be violated by accident during implementation": capacity is
// declared-only, never inferred from surviving instance count. CLAUDE.md §9 states the
// exact rule this file must not violate:
//
//	"Capacity ≠ count. Instance count is a redundancy fact, derivable from the graph
//	(how many failure domains exist). Node capacity is a load fact, only ever from the
//	declaration (can survivors carry peak). Never treat '3 instances' as '3× capacity'.
//	Absent declared capacity → the tier is capacity_unknown → any capacity finding is
//	not_assessable, never assumed, never zero."
//
// The function below is written so that violating this rule would require deleting the
// one guard clause that enforces it — not something an incremental change could do by
// accident while "just wiring in the real numbers."
//
// This package has no dependency on preflight/core (core imports analyse, not the
// reverse — see core/assess.go), so results here are plain (value, ok, reason) tuples
// rather than core.Assessment[T]; core/assess.go's thin wrapper is what turns the
// not_assessable/reason pairing below into a real core.Assessment[float64] with real
// core.Provenance attached.
package analyse

// SurvivingCapacity computes the aggregate capacity a node type's surviving instances
// can carry, GIVEN a declared per-instance capacity value — it never derives that
// per-instance number from anything else. capacityKey names the entry in
// core.Workload.Capacity to look up (e.g. "app_node_rps", golden/workload.yaml's own
// example) — this function does not invent a node-type-to-key naming convention, since
// none is specified anywhere available; the caller supplies the exact key the workload
// declaration actually uses.
//
// survivingInstances answers a DIFFERENT, already-answerable question (redundancy,
// derivable from the graph — how many independent copies of this node type remain
// after a failure) and is accepted as a parameter precisely so this function's only
// job is the capacity half, never the counting half — the two must never be computed
// by the same code path, so a future change to the counting logic cannot accidentally
// also change what "declared capacity" means.
//
// Returns (value, true, "") when capacity was declared, or (0, false, reason) when it
// was not — the caller (core/assess.go) turns the latter into not_assessable, never a
// silent zero.
func SurvivingCapacity(declaredCapacity map[string]float64, capacityKey string, survivingInstances int) (value float64, ok bool, notAssessableReason string) {
	perInstance, declared := declaredCapacity[capacityKey]
	if !declared {
		// THE guard clause. Every other line in this file exists to feed this
		// function correctly; this line is what actually enforces CLAUDE.md §9. If
		// this check is ever removed or bypassed, the function reverts to exactly
		// the anti-pattern the rule exists to prevent — a fact this comment states
		// explicitly so a future editor sees the stakes before touching it.
		return 0, false, "capacity_unknown: \"" + capacityKey + "\" was not declared in workload.yaml — " +
			"surviving instance count alone is never treated as a capacity value (CLAUDE.md §9)"
	}
	return perInstance * float64(survivingInstances), true, ""
}
