// This file holds RPOFeasibility/RTOFeasibility — provider-agnostic pure
// classification logic over an already-derived ReplicationMode/FailoverMechanism.
//
// PC-22 relocation, stated rather than silently done: this file used to ALSO derive
// ReplicationMode/FailoverMechanism itself (RDSFailoverCapability,
// ElastiCacheFailoverCapability, DeriveNodeReplicationAndFailover) — moved out to
// providers/aws's own FailoverMapping data + ingest/build.go's deriveFailover,
// because that logic's OUTPUT TEXT ("Multi-AZ automatic failover to a synchronous
// standby replica") is real, provider-specific, doc-verified knowledge, not
// provider-agnostic classification — exactly the kind of thing this package's own
// doc comment (package capacity.go's own note) says does not belong here. Found while
// starting PC-22's Azure work, before any Azure mapping was written: the old
// DeriveNodeReplicationAndFailover dispatched by canonical node_type alone
// (managed_database), so an Azure SQL node would have silently received AWS RDS's own
// wording verbatim — a factually wrong claim about Azure, not merely an imprecise one.
//
// Like capacity.go, this package has no dependency on preflight/core (core imports
// analyse, not the reverse — see core/assess.go): RPOFeasibility/RTOFeasibility return
// plain (value, ok, reason) tuples.
package analyse

// FailoverMechanismNone is the exact, canonical sentinel meaning "no automatic
// failover mechanism exists" — re-exported as core.FailoverMechanismNone (core/
// assess.go) so ingest/build.go, which cannot import this internal package directly,
// still shares this exact value rather than a second, independently-typed copy that
// could drift. Found the hard way once already: an earlier version had each producer
// return an elaborated sentence STARTING WITH "none" (e.g. "none — no standby
// exists..."), while RTOFeasibility did an exact match against the bare string "none"
// — since the two never matched, RTOFeasibility silently reported Assessed(true) (a
// failover path exists) for a database that had none at all. Caught by
// TestRTORPOFeasibility_AgainstGoldenBundles against the real broken bundle, not by a
// unit test using matched fixtures. This constant remains the one place either side
// (now ingest as producer, this file's RTOFeasibility as consumer) can go out of sync.
const FailoverMechanismNone = "none"

// RPOFeasibility answers a narrow, structurally-defensible question: given a declared
// RPO target (seconds) and a resource's derived ReplicationMode, is that target
// KNOWABLY feasible or infeasible from replication mode alone?
//
// Deliberately narrow, not a general RPO calculator: synchronous replication can
// satisfy ANY declared RPO (including 0) by definition — no measurement needed to
// know that. Asynchronous replication CANNOT satisfy RPO=0 by definition (some window
// of unreplicated writes always exists) — also knowable without measurement. Any other
// combination (async + a non-zero target, or an unknown replication mode) requires
// knowing the ACTUAL replication lag, which is a Rung-3 observed fact, not something
// Layer 1 structural analysis can determine — those cases return ok=false, not a guess.
func RPOFeasibility(declaredRPOSeconds *float64, replicationMode *string) (value bool, ok bool, notAssessableReason string) {
	if declaredRPOSeconds == nil {
		return false, false, "no rpo_seconds requirement declared in workload.yaml"
	}
	if replicationMode == nil {
		return false, false, "replication mode is unknown for this resource — cannot assess RPO feasibility"
	}
	switch *replicationMode {
	case "sync":
		return true, true, "" // sync satisfies any declared RPO >= 0, including 0, by definition
	case "async":
		if *declaredRPOSeconds == 0 {
			return false, true, "" // async can never guarantee zero data loss, by definition
		}
		return false, false, "declared RPO is non-zero and replication is async — whether the actual replication lag meets this specific target requires observed evidence (Rung 3), not structural analysis alone"
	default:
		return false, false, "replication mode \"" + *replicationMode + "\" has no known RPO feasibility rule"
	}
}

// RTOFeasibility answers what Layer 1 structural analysis can honestly answer about
// recovery time: whether an automatic failover mechanism exists at all. It does NOT
// predict a failover duration in seconds and must never be read as satisfying a
// declared rto_seconds target precisely — per CLAUDE.md §6, precise timing prediction
// is Layer 2/3's job (scenario/discrete-event simulation, P2), not Layer 1's (PC-14).
// This function only ever answers "does a failover PATH exist," never "in how long" —
// enforced by its own signature (no rto_seconds parameter at all), not by convention.
func RTOFeasibility(failoverMechanism *string) (value bool, ok bool, notAssessableReason string) {
	if failoverMechanism == nil {
		return false, false, "failover mechanism is unknown for this resource"
	}
	if *failoverMechanism == FailoverMechanismNone {
		return false, true, ""
	}
	return true, true, ""
}

// Requirement is analyse's own minimal, core-independent counterpart to
// core.Requirement — just enough shape (ID, Value) for RequirementValue to work
// without importing preflight/core.
type Requirement struct {
	ID    string
	Value any
}

// RequirementValue extracts a numeric value for the requirement named id from
// requirements, or (0, false) if no such requirement is present. YAML unmarshals a
// bare integer literal (e.g. "60") into an int, and a decimal literal (e.g. "99.95")
// into a float64, when the target field is `any` — both are handled so this doesn't
// silently miss integer-valued requirements like rto_seconds/rpo_seconds.
func RequirementValue(requirements []Requirement, id string) (float64, bool) {
	for _, r := range requirements {
		if r.ID != id {
			continue
		}
		switch v := r.Value.(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		default:
			return 0, false
		}
	}
	return 0, false
}
