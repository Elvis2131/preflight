package core

import (
	"sort"

	"preflight/core/internal/analyse"
)

// Fault is one declared failure condition for /simulate (PC-82). Only "region_loss"
// is hand-verified in this version — see Simulate's own doc comment for the full
// scope statement.
type Fault struct {
	Type   string `json:"type" validate:"required" jsonschema:"required"`
	Target string `json:"target" validate:"required" jsonschema:"required"`

	// DestinationCIDR is PC-129's own addition — required only for "route_removal":
	// the specific route's own destination_cidr on the named route table (Target).
	// Empty for every other fault type. Not part of any of the six frozen contracts
	// (contracts/CHANGELOG.md), so this is a plain additive field, no version bump.
	DestinationCIDR string `json:"destination_cidr,omitempty" jsonschema:"description=Required only for route_removal — the destination_cidr of the specific route to remove from Target (a route table ID). Empty for every other fault type."`

	// IAMPolicyID/IAMRemoveStatementSid/IAMAddDenyStatement are PC-135's own addition
	// — required only for "iam_policy_change": Target names the policy document
	// (see core/iam_fault.go's own doc comment for the three places a policy ID can
	// come from) to mutate. Exactly one of IAMRemoveStatementSid or
	// IAMAddDenyStatement must be set — the Card's own two named mutations, "remove a
	// statement, add a deny" — never both, never neither.
	IAMRemoveStatementSid string           `json:"iam_remove_statement_sid,omitempty" jsonschema:"description=Required only for iam_policy_change when removing a statement — the Sid of the statement to remove from the policy document named by Target."`
	IAMAddDenyStatement   *PolicyStatement `json:"iam_add_deny_statement,omitempty" jsonschema:"description=Required only for iam_policy_change when adding a deny — a new statement appended to the policy document named by Target. Effect is always forced to Deny regardless of what is supplied here, since this fault only ever injects a new restriction."`

	// SGRuleRemove/SGRuleAdd/NACLRuleRemove/NACLRuleAdd are PC-130's own addition —
	// required only for "sg_rule_change"/"nacl_rule_change": Target names the
	// security-group or NACL node to mutate. Exactly one of the Remove/Add pair for
	// the matching fault type must be set, same "never both, never neither"
	// discipline as IAMRemoveStatementSid/IAMAddDenyStatement above.
	SGRuleRemove   *SGRule   `json:"sg_rule_remove,omitempty" jsonschema:"description=Required only for sg_rule_change when removing a rule — the exact rule (direction, protocol, ports, CIDRs/source SG) to remove from the security group named by Target."`
	SGRuleAdd      *SGRule   `json:"sg_rule_add,omitempty" jsonschema:"description=Required only for sg_rule_change when adding a rule — appended verbatim to the security group named by Target."`
	NACLRuleRemove *NACLRule `json:"nacl_rule_remove,omitempty" jsonschema:"description=Required only for nacl_rule_change when removing a rule — the exact rule to remove from the NACL named by Target."`
	NACLRuleAdd    *NACLRule `json:"nacl_rule_add,omitempty" jsonschema:"description=Required only for nacl_rule_change when adding a rule — appended verbatim to the NACL named by Target."`
	// DeregisterTarget is required only for "target_deregistration": Target names the
	// load balancer, DeregisterTarget the registered target node ID to drop from it.
	DeregisterTarget string `json:"deregister_target,omitempty" jsonschema:"description=Required only for target_deregistration — the node ID of the registered target to drop from the load balancer named by Target."`
}

// Journey is one entry-point-to-stateful-node path's fate under the declared faults —
// PC-82's own response shape names "journeys[]" without further detail (the source
// PRD/Design text was unavailable when this was built), so the cross product of every
// surviving entry point against every stateful node is the design decision made here:
// it is the most informative shape ("which specific paths survive") without inventing
// a narrower concept the ticket never named.
type Journey struct {
	EntryPoint string `json:"entry_point"`
	Target     string `json:"target"`
	Survives   bool   `json:"survives"`
}

// SimulateResponse is /simulate's full response shape (PC-82: "{journeys[], capacity,
// severed_paths[], cascade[], verdict}", PRD §6 — verbatim from the ticket's own
// Confirmation criterion, since the referenced PRD/Design doc pages were unavailable).
type SimulateResponse struct {
	Journeys     []Journey          `json:"journeys"`
	Capacity     AssessmentEnvelope `json:"capacity"`
	SeveredPaths []string           `json:"severed_paths"`
	Cascade      []string           `json:"cascade"`
	Verdict      AssessmentEnvelope `json:"verdict"`

	// FlowDetail is PC-125/PC-129's own real, per-declared-journey structural flow
	// (core.ComputeAllJourneyFlows) against the fault-mutated IR — strictly more
	// precise than Journeys above (which only sees generic graph connectivity, not a
	// real SG/NACL/route decision). Empty (never nil, same "no null in JSON"
	// discipline as Journeys/SeveredPaths/Cascade) when the workload declares no
	// journeys at all.
	FlowDetail []JourneyFlowResult `json:"flow_detail"`

	// Load is PC-127's own addition — PC-126's real per-component utilization
	// (ComputeComponentLoad/RankBottlenecks) against the SAME fault-mutated IR/killed
	// set FlowDetail already reflects, so a Simulate-mode UI can show "before" and
	// "after" utilization for one fault without a second round-trip or reimplementing
	// the load engine. Empty (never nil) when the workload declares no journeys.
	Load []ComponentLoad `json:"load"`

	// Latency is PC-128's own addition — Layer 3 latency/saturation under the SAME
	// fault-mutated IR and killed set as FlowDetail/Load, one entry per declared
	// journey: assessed (conditional on the listed declared inputs) or not_assessable
	// naming what is missing. Empty (never nil) when the workload declares no journeys.
	Latency []JourneyLatency `json:"latency"`
}

// Simulate applies a declared fault set against an IR + Workload, reusing PC-14's
// existing fault-injection engines (SimulateLoss for forward reachability from entry
// points, SurvivingCapacity for the declared-capacity check) rather than duplicating
// them — PC-82's own acceptance criterion, verbatim: "reuses PC-14's existing
// fault-injection logic ... rather than duplicating it — this is the same engine, a
// different entry point."
//
// SCOPE, stated explicitly per PC-82's own Confirmation criterion ("declaring a
// region_loss fault against the golden architecture produces a real, hand-verified
// verdict" — not every conceivable fault type): two fault types are implemented and
// hand-verified here, "region_loss" and "node_loss".
//
// "region_loss": region is not a modeled containment level in the IR — no node
// carries a "region" attribute; AWS subnets carry availability_zone, one level below
// region, and Azure has no per-node AZ attribute at all (see golden/azure/README.md).
// So this answers the only version of "region loss" the IR can honestly support
// today: if the declared workload names EXACTLY the target region and has no OTHER
// declared region, the entire graph is treated as killed — there is no secondary
// region for anything to fail over into, which is a real, correct answer for a
// single-region workload (golden/workload.yaml declares exactly one region), not an
// approximation of a partial regional kill this IR cannot actually compute.
//
// "node_loss" (PC-88): kills exactly the named node ID, nothing else — added when the
// canvas's "click a node and kill it" interaction needed a fault type /simulate could
// honestly answer at that granularity, one that region_loss's whole-graph semantics
// can't provide. Reuses the identical SimulateLoss/SurvivingCapacity engine below —
// killedNodesForFaults only decides WHICH node IDs are in the killed set, never how
// reachability or capacity is computed from that set, so this is genuinely "the same
// engine, a different entry point," not new simulation logic layered on top of it.
//
// "nat_gateway_loss" (PC-129): kills the named NAT gateway node — a real, distinct IR
// node, so this reuses the identical node_loss mechanism (a NAT gateway failing is
// honestly "this node is gone," the same fact node_loss already expresses), not a new
// kind of kill.
//
// "route_removal" (PC-129): Target names a route table, DestinationCIDR names the
// specific route on it — this is an EDGE mutation, not a node kill, so it cannot be
// expressed via the killed-node-set mechanism at all. core.WithRouteRemoved produces
// a mutated COPY of ir (the specific route retargeted to a sentinel that resolves to
// nothing — see that function's own doc comment for why retargeting, not deletion);
// Simulate runs its entire pipeline against that mutated copy instead of the original.
//
// Both new fault types additionally populate FlowDetail (PC-125's real per-journey
// structural flow, itself built on PC-114's request engine) — this is computed for
// EVERY fault type, not specially cased, since it is strictly more precise than the
// pre-existing entryPoint×statefulNode Journey cross-product (which only sees generic
// graph connectivity, not real SG/NACL/route decisions).
//
// Any other fault type, a region_loss target that doesn't match the workload's own
// declared region(s), a node_loss/nat_gateway_loss target that doesn't exist in the
// IR, or a route_removal target route table that doesn't exist, returns
// not_assessable with a stated reason rather than guessing — never a fabricated
// partial result.
func Simulate(ir *IR, workload Workload, faults []Fault, prov Provenance) SimulateResponse {
	mutatedIR, killed, ok, reason := resolveFaults(ir, workload, faults)
	if !ok {
		na := NotAssessable[any](reason, prov).ToEnvelope()
		return SimulateResponse{Journeys: make([]Journey, 0), SeveredPaths: make([]string, 0), Cascade: make([]string, 0), FlowDetail: make([]JourneyFlowResult, 0), Load: make([]ComponentLoad, 0), Latency: make([]JourneyLatency, 0), Capacity: na, Verdict: na}
	}
	ir = mutatedIR

	var allEdges []DirectedEdge
	for _, e := range ir.Edges {
		allEdges = append(allEdges, DirectedEdge{From: e.From, To: e.To})
	}

	var entryPoints, statefulNodes []string
	for _, n := range ir.Nodes {
		switch n.Type {
		case NodeTypeDNS, NodeTypeLoadBalancer:
			if !killed[n.ID] {
				entryPoints = append(entryPoints, n.ID)
			}
		case NodeTypeManagedDatabase, NodeTypeCache, NodeTypeQueueStream, NodeTypeObjectStore:
			statefulNodes = append(statefulNodes, n.ID)
		}
	}
	sort.Strings(entryPoints)
	sort.Strings(statefulNodes)

	unreachable := map[string]bool{}
	if len(entryPoints) > 0 {
		for _, id := range SimulateLoss(allEdges, entryPoints, killed) {
			unreachable[id] = true
		}
	}

	// journeys/severedPaths are initialized non-nil, deliberately, like cascade below
	// — a Go nil slice marshals to JSON `null`, not `[]`, and every consumer of this
	// wire response reasonably expects an array it can iterate/range over
	// unconditionally. Guarding against `null` only in one TS consumer (as this
	// codebase briefly did) is exactly the symptomatic fix PC-83 already rejected
	// elsewhere — the real fix is here, at the one place that produces the response,
	// so it's fixed for every current and future caller (a CLI script, the
	// "simulate" MCP tool's own real output schema — PC-93 — anything else that
	// calls /simulate directly).
	journeys := make([]Journey, 0)
	severedSet := map[string]bool{}
	for _, target := range statefulNodes {
		survives := len(entryPoints) > 0 && !killed[target] && !unreachable[target]
		if !survives {
			severedSet[target] = true
		}
		for _, ep := range entryPoints {
			journeys = append(journeys, Journey{EntryPoint: ep, Target: target, Survives: survives})
		}
	}

	cascadeSet := map[string]bool{}
	for id := range killed {
		cascadeSet[id] = true
	}
	for id := range unreachable {
		cascadeSet[id] = true
	}

	// Ordered by hop distance from the point of failure (PC-89), not alphabetically —
	// see analyse.CascadeOrder's own doc comment for why this is a structural fact, not
	// a timing claim. severedPaths is ordered the same way since it's always a subset
	// of cascade: the same "how far from the failure" story applies to both.
	cascade := analyse.CascadeOrder(allEdges, killed, cascadeSet)
	severedPaths := analyse.CascadeOrder(allEdges, killed, severedSet)

	survivingWorkloadInstances := 0
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeContainerWorkload && !killed[n.ID] {
			survivingWorkloadInstances++
		}
	}
	capValue, capOK, capReason := analyse.SurvivingCapacity(workload.Capacity, "app_node_rps", survivingWorkloadInstances)
	var capacity AssessmentEnvelope
	if !capOK {
		capacity = NotAssessable[any](capReason, prov).ToEnvelope()
	} else {
		capacity = Assessed[any](capValue, prov).ToEnvelope()
	}

	verdict := deriveVerdict(len(entryPoints), len(statefulNodes), len(severedPaths), prov)

	// FlowDetail (PC-125/PC-129): real, per-declared-journey structural flow against
	// the (possibly fault-mutated) IR — always computed when the workload declares
	// journeys, for every fault type, not specially cased to the two new ones.
	flowDetail := make([]JourneyFlowResult, 0)
	load := make([]ComponentLoad, 0)
	latency := make([]JourneyLatency, 0)
	if len(workload.Journeys) > 0 {
		flowDetail = ComputeAllJourneyFlows(ir, workload, killed)
		load = RankBottlenecks(ComputeComponentLoad(ir, workload, killed))
		latency = ComputeDegradedLatency(ir, workload, killed, prov)
	}

	return SimulateResponse{
		Journeys:     journeys,
		Capacity:     capacity,
		SeveredPaths: severedPaths,
		Cascade:      cascade,
		Verdict:      verdict,
		FlowDetail:   flowDetail,
		Load:         load,
		Latency:      latency,
	}
}

// resolveFaults is killedNodesForFaults, extended (PC-129) to also produce a mutated
// IR copy for edge-level faults (route_removal) that cannot be expressed as a killed
// node set at all. Every pre-existing fault type's own behavior is unchanged — this
// only adds two new cases and threads the (possibly mutated) IR through.
func resolveFaults(ir *IR, workload Workload, faults []Fault) (mutatedIR *IR, killed map[string]bool, ok bool, reason string) {
	mutatedIR = ir
	for _, f := range faults {
		switch f.Type {
		case "route_removal":
			if !nodeExists(mutatedIR, f.Target) {
				return nil, nil, false, "route_removal target \"" + f.Target + "\" (a route table) does not exist in this IR — refusing to guess which route table was meant"
			}
			if f.DestinationCIDR == "" {
				return nil, nil, false, "route_removal requires destination_cidr — refusing to guess which route on \"" + f.Target + "\" was meant"
			}
			mutatedIR = WithRouteRemoved(mutatedIR, f.Target, f.DestinationCIDR)
		case "iam_policy_change":
			hasRemove := f.IAMRemoveStatementSid != ""
			hasAdd := f.IAMAddDenyStatement != nil
			if hasRemove == hasAdd {
				return nil, nil, false, "iam_policy_change requires exactly one of iam_remove_statement_sid or iam_add_deny_statement, not both or neither"
			}
			var mutated *IR
			var ok bool
			if hasRemove {
				mutated, ok = WithIAMStatementRemoved(mutatedIR, f.Target, f.IAMRemoveStatementSid)
				if !ok {
					return nil, nil, false, "iam_policy_change target \"" + f.Target + "\" (a policy document ID) or statement Sid \"" + f.IAMRemoveStatementSid + "\" does not exist in this IR — refusing to guess which statement was meant"
				}
			} else {
				mutated, ok = WithIAMDenyStatementAdded(mutatedIR, f.Target, *f.IAMAddDenyStatement)
				if !ok {
					return nil, nil, false, "iam_policy_change target \"" + f.Target + "\" (a policy document ID) does not exist in this IR — refusing to guess which policy was meant"
				}
			}
			mutatedIR = mutated
		case "sg_rule_change":
			hasRemove := f.SGRuleRemove != nil
			hasAdd := f.SGRuleAdd != nil
			if hasRemove == hasAdd {
				return nil, nil, false, "sg_rule_change requires exactly one of sg_rule_remove or sg_rule_add, not both or neither"
			}
			var mutated *IR
			var ok bool
			if hasRemove {
				mutated, ok = WithSGRuleRemoved(mutatedIR, f.Target, *f.SGRuleRemove)
				if !ok {
					return nil, nil, false, "sg_rule_change target \"" + f.Target + "\" does not exist, or has no rule exactly matching sg_rule_remove — refusing to guess which rule was meant"
				}
			} else {
				mutated, ok = WithSGRuleAdded(mutatedIR, f.Target, *f.SGRuleAdd)
				if !ok {
					return nil, nil, false, "sg_rule_change target \"" + f.Target + "\" (a security group) does not exist in this IR"
				}
			}
			mutatedIR = mutated
		case "target_deregistration":
			mutated, ok := WithTargetDeregistered(mutatedIR, f.Target, f.DeregisterTarget)
			if !ok {
				return nil, nil, false, "target_deregistration target \"" + f.Target + "\" is not a load balancer with a modelled registration of \"" + f.DeregisterTarget + "\" (registrations come from canvas-authored targets or HCL aws_lb_target_group_attachment resources; targets registered by an Auto Scaling group, ECS service or controller are not visible) — refusing to guess"
			}
			mutatedIR = mutated
		case "nacl_rule_change":
			hasRemove := f.NACLRuleRemove != nil
			hasAdd := f.NACLRuleAdd != nil
			if hasRemove == hasAdd {
				return nil, nil, false, "nacl_rule_change requires exactly one of nacl_rule_remove or nacl_rule_add, not both or neither"
			}
			var mutated *IR
			var ok bool
			if hasRemove {
				mutated, ok = WithNACLRuleRemoved(mutatedIR, f.Target, *f.NACLRuleRemove)
				if !ok {
					return nil, nil, false, "nacl_rule_change target \"" + f.Target + "\" does not exist, or has no rule exactly matching nacl_rule_remove — refusing to guess which rule was meant"
				}
			} else {
				mutated, ok = WithNACLRuleAdded(mutatedIR, f.Target, *f.NACLRuleAdd)
				if !ok {
					return nil, nil, false, "nacl_rule_change target \"" + f.Target + "\" (a NACL) does not exist in this IR"
				}
			}
			mutatedIR = mutated
		}
	}
	killed, ok, reason = killedNodesForFaults(mutatedIR, workload, faults)
	if !ok {
		return nil, nil, false, reason
	}
	return mutatedIR, killed, true, ""
}

// killedNodesForFaults resolves a fault list into the concrete set of killed node
// IDs. See Simulate's own doc comment for the full scope statement on why only
// region_loss is implemented.
func killedNodesForFaults(ir *IR, workload Workload, faults []Fault) (killed map[string]bool, ok bool, reason string) {
	killed = map[string]bool{}
	for _, f := range faults {
		switch f.Type {
		case "region_loss":
			if len(workload.Regions) != 1 || workload.Regions[0] != f.Target {
				return nil, false, "region_loss target \"" + f.Target + "\" does not match the workload's own single declared region — multi-region topologies are not modeled in the IR (no per-node region attribute exists), so a partial regional kill cannot be honestly computed"
			}
			for _, n := range ir.Nodes {
				killed[n.ID] = true
			}
		case "node_loss":
			if !nodeExists(ir, f.Target) {
				return nil, false, "node_loss target \"" + f.Target + "\" does not exist in this IR — refusing to guess which node was meant"
			}
			killed[f.Target] = true
		case "external_dependency_outage":
			// PC-131: take an external_dependency node (payment rail, identity
			// provider, third-party API) down. Only a node that really IS an
			// external_dependency is accepted — refusing to guess, same as every other
			// fault here, since "outage" of an ordinary component is node_loss.
			n, found := nodeByID(ir, f.Target)
			if !found {
				return nil, false, "external_dependency_outage target \"" + f.Target + "\" does not exist in this IR — refusing to guess which dependency was meant"
			}
			if n.Type != NodeTypeExternalDependency {
				return nil, false, "external_dependency_outage target \"" + f.Target + "\" is a " + string(n.Type) + ", not an external_dependency — use node_loss for an ordinary component"
			}
			killed[f.Target] = true
		case "nat_gateway_loss":
			if !nodeExists(ir, f.Target) {
				return nil, false, "nat_gateway_loss target \"" + f.Target + "\" does not exist in this IR — refusing to guess which NAT gateway was meant"
			}
			killed[f.Target] = true
		case "route_removal", "iam_policy_change", "sg_rule_change", "nacl_rule_change", "target_deregistration":
			// Already applied as an IR mutation by resolveFaults, before ir (this
			// function's own parameter) was even built — nothing to add to the
			// killed-node set for any of these; ir already reflects the change.
		default:
			return nil, false, "fault type \"" + f.Type + "\" is not implemented — only region_loss, node_loss, nat_gateway_loss, external_dependency_outage, route_removal, iam_policy_change, sg_rule_change, nacl_rule_change, and target_deregistration are hand-verified in this version (PC-82, PC-88, PC-129, PC-130, PC-131, PC-135)"
		}
	}
	return killed, true, ""
}

func nodeByID(ir *IR, id string) (Node, bool) {
	for _, n := range ir.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

func nodeExists(ir *IR, id string) bool {
	for _, n := range ir.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

// deriveVerdict classifies the outcome from what Simulate already computed — a real
// derived conclusion (CLAUDE.md §5 forbids FABRICATION, not derivation from a fact
// this function already has in hand). Three values, mirroring this codebase's
// established discipline of small, named, non-numeric outcome vocabularies rather
// than an invented score: "total_outage" (no entry point survives, or every stateful
// target is severed), "degraded" (some but not all severed), "unaffected" (none
// severed — including the vacuous case of no stateful targets in the graph at all).
func deriveVerdict(survivingEntryPoints, totalStatefulTargets, severedCount int, prov Provenance) AssessmentEnvelope {
	switch {
	case survivingEntryPoints == 0 || (totalStatefulTargets > 0 && severedCount == totalStatefulTargets):
		return Assessed[any]("total_outage", prov).ToEnvelope()
	case severedCount > 0:
		return Assessed[any]("degraded", prov).ToEnvelope()
	default:
		return Assessed[any]("unaffected", prov).ToEnvelope()
	}
}
