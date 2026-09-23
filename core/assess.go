// Package core's own doc comment already states the architectural rule this file
// exists to satisfy: "core is the only public surface onto core/internal/{ir,analyse,
// simulate}". core/internal/analyse has no dependency on this package at all (its
// functions take and return plain Go types) — this file is where the wrapping into
// core's own Provenance/Assessment[T]/Node/Workload types actually happens, which is
// what lets ingest/, server/, and tests exercising the real pipeline end to end call
// into PC-11/PC-14's analysis functions, without any of them being able to reach
// core/internal/analyse directly (compiler-enforced — see core/boundary_test.go).
package core

import "preflight/core/internal/analyse"

// DirectedEdge, MinVertexCutResult mirror core/internal/analyse's own types exactly —
// re-exported (not merely re-declared) so a caller never needs to know the internal
// package exists at all.
type DirectedEdge = analyse.DirectedEdge
type MinVertexCutResult = analyse.MinVertexCutResult

// MinVertexCut — see core/internal/analyse.MinVertexCut's doc comment for the full
// contract (determinism guarantee, the Uncuttable outcome, etc.). Pure pass-through:
// MinVertexCut takes no core types at all.
func MinVertexCut(edges []DirectedEdge, source, sink string) MinVertexCutResult {
	return analyse.MinVertexCut(edges, source, sink)
}

// AssessNode is the core.Node-shaped wrapper around PC-11's AssessResolvable — this IS
// the original AssessNode PC-11 delivered (same behavior, same acceptance criteria),
// relocated here so core/internal/analyse can remain free of any core import (see this
// file's own doc comment for why that matters).
func AssessNode[T any](node Node, compute func(Node) T) Assessment[T] {
	el := analyse.ResolvableElement{
		ID:               node.ID,
		Unresolved:       node.Resolution == ResolutionUnresolved,
		ProvenanceReason: node.Provenance.Reason,
	}
	value, ok, reason := analyse.AssessResolvable(el, func() T { return compute(node) })
	if !ok {
		return NotAssessable[T](reason, node.Provenance)
	}
	return Assessed(value, node.Provenance)
}

// AssessEdge is AssessNode's counterpart for edges — see AssessNode's doc.
func AssessEdge[T any](edge Edge, compute func(Edge) T) Assessment[T] {
	el := analyse.ResolvableElement{
		ID:               edge.ID,
		Unresolved:       edge.Resolution == ResolutionUnresolved,
		ProvenanceReason: edge.Provenance.Reason,
	}
	value, ok, reason := analyse.AssessResolvable(el, func() T { return compute(edge) })
	if !ok {
		return NotAssessable[T](reason, edge.Provenance)
	}
	return Assessed(value, edge.Provenance)
}

// SurvivingCapacity wraps core/internal/analyse.SurvivingCapacity's plain tuple result
// into a real core.Assessment[float64] with real core.Provenance attached — this is
// CLAUDE.md §9's capacity-discipline rule's only sanctioned entry point.
func SurvivingCapacity(declaredCapacity map[string]float64, capacityKey string, survivingInstances int, prov Provenance) Assessment[float64] {
	value, ok, reason := analyse.SurvivingCapacity(declaredCapacity, capacityKey, survivingInstances)
	if !ok {
		return NotAssessable[float64](reason, prov)
	}
	return Assessed(value, prov)
}

// FailoverMechanismNone re-exports core/internal/analyse's own sentinel — the same
// alias pattern already used for DeltaKind's values (see scorecard.go) — so ingest/
// (which cannot import core/internal/analyse itself; Go's internal/ convention
// restricts that to core's own tree) has one shared source for this literal rather
// than a second, independently-typed copy that could drift, exactly the failure mode
// this sentinel exists to prevent (see analyse.FailoverMechanismNone's own doc
// comment).
const FailoverMechanismNone = analyse.FailoverMechanismNone

// DeriveNodeReplicationAndFailover returns a node's pre-derived replication mode and
// failover mechanism (PC-22: relocated from core/internal/analyse's own derivation
// logic to ingest/build.go + providers/*'s own FailoverMapping data — see that type's
// doc comment for why: the mechanism WORDING is provider-specific, doc-verified
// knowledge, not provider-agnostic classification logic, so it belongs in providers/
// as data, the same as everything else that package holds). This is now a plain
// accessor, not a derivation — ingest already computed the real values once, at parse
// time, from the correct provider's own mapping.
func DeriveNodeReplicationAndFailover(node Node) (replicationMode, failoverMechanism *string) {
	if node.Capability == nil {
		return nil, nil
	}
	return node.Capability.ReplicationMode, node.Capability.FailoverMechanism
}

// RPOFeasibility, RTOFeasibility wrap core/internal/analyse's plain-tuple results into
// real core.Assessment[bool]s. RTOFeasibility answers only whether a failover PATH
// exists, never a timing prediction — that boundary is enforced by the underlying
// function's own signature (no rto_seconds parameter), not by convention.
func RPOFeasibility(declaredRPOSeconds *float64, replicationMode *string, prov Provenance) Assessment[bool] {
	value, ok, reason := analyse.RPOFeasibility(declaredRPOSeconds, replicationMode)
	if !ok {
		return NotAssessable[bool](reason, prov)
	}
	return Assessed(value, prov)
}

func RTOFeasibility(failoverMechanism *string, prov Provenance) Assessment[bool] {
	value, ok, reason := analyse.RTOFeasibility(failoverMechanism)
	if !ok {
		return NotAssessable[bool](reason, prov)
	}
	return Assessed(value, prov)
}

// RequirementValue wraps core/internal/analyse's plain-Requirement version with the
// real core.Workload.
func RequirementValue(w Workload, id string) (float64, bool) {
	reqs := make([]analyse.Requirement, len(w.Requirements))
	for i, r := range w.Requirements {
		reqs[i] = analyse.Requirement{ID: r.ID, Value: r.Value}
	}
	return analyse.RequirementValue(reqs, id)
}

// SPOFCandidate mirrors core/internal/analyse's own type exactly.
type SPOFCandidate = analyse.SPOFCandidate

// DetectSPOFs — see core/internal/analyse.DetectSPOFs's doc comment. entryPoints and
// criticalNodes are node IDs (core.Node.ID), not core.Node values, since the
// underlying algorithm only needs the graph's edge structure — building the edge list
// itself (from an IR's Nodes/Edges, respecting resolution state) is the caller's job,
// not this function's.
func DetectSPOFs(edges []DirectedEdge, entryPoints, criticalNodes []string) []SPOFCandidate {
	return analyse.DetectSPOFs(edges, entryPoints, criticalNodes)
}

// SimulateLoss — see core/internal/analyse.SimulateLoss's doc comment. edges is the
// graph structure; entryPoints and killed are node IDs. Returns the sorted list of
// node IDs that become unreachable as a consequence of the kill (never including the
// killed nodes themselves — see the underlying function's doc comment for why those
// are a distinct fact).
func SimulateLoss(edges []DirectedEdge, entryPoints []string, killed map[string]bool) []string {
	return analyse.SimulateLoss(edges, entryPoints, killed)
}

// ContainmentBlastRadius — see core/internal/analyse.ContainmentBlastRadius's doc
// comment. containmentEdges should be filtered to EdgeTypeContainedIn edges only —
// this function does not filter for the caller, since mixing in other edge types
// would answer a different question (see the underlying function's doc comment).
func ContainmentBlastRadius(containmentEdges []DirectedEdge, killedNode string) []string {
	return analyse.ContainmentBlastRadius(containmentEdges, killedNode)
}

// DeriveLikelihood, DeriveDetectability, DeriveImpact — see core/internal/analyse's
// doc comments. Each wraps a plain-tuple result into a real Assessment[string] with
// real Provenance, per this file's established pattern.
func DeriveLikelihood(prov Provenance) Assessment[string] {
	value, ok, reason := analyse.DeriveLikelihood()
	if !ok {
		return NotAssessable[string](reason, prov)
	}
	return Assessed(value, prov)
}

func DeriveDetectability(detection DetectionState, prov Provenance) Assessment[string] {
	value, ok, reason := analyse.DeriveDetectability(string(detection))
	if !ok {
		return NotAssessable[string](reason, prov)
	}
	return Assessed(value, prov)
}

func DeriveImpact(blastRadiusSize int, workloadCriticality string, prov Provenance) Assessment[string] {
	value, ok, reason := analyse.DeriveImpact(blastRadiusSize, workloadCriticality)
	if !ok {
		return NotAssessable[string](reason, prov)
	}
	return Assessed(value, prov)
}

// StorageEncryptionCheck wraps core/internal/analyse's plain-tuple check into a real
// ComplianceResult with real Provenance — PC-18's one implemented control, generalized
// (PC-29) to run against any managed_database node's Capability.EncryptionMechanism
// rather than only AWS RDS's raw storage_encrypted attribute — see that function's own
// doc comment for the full account of what generalizes across clouds and what doesn't.
func StorageEncryptionCheck(encrypted *bool, prov Provenance) (result ComplianceResult, rationale string) {
	status, reason := analyse.StorageEncryptionCheck(encrypted)
	return ComplianceResult{Status: ComplianceStatus(status), Provenance: prov}, reason
}

// NATGatewayRedundancyCheck wraps core/internal/analyse's plain-tuple check into a
// real ComplianceResult with real Provenance.
func NATGatewayRedundancyCheck(natGatewayCountBySubnet map[string]int, prov Provenance) (result ComplianceResult, rationale string) {
	status, reason := analyse.NATGatewayRedundancyCheck(natGatewayCountBySubnet)
	return ComplianceResult{Status: ComplianceStatus(status), Provenance: prov}, reason
}
