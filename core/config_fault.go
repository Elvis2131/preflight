// This file is PC-130: configuration-change faults — "the infrastructure is intact
// but a rule changed," a different failure class from PC-14/78/129's own "a thing
// disappears" faults, invisible to topology-only analysis. Two fault types are
// implemented here, both hand-verified against the real golden bundle:
// sg_rule_change and nacl_rule_change. Both reuse the SAME copy-on-write IR-mutation
// discipline PC-129/135 established (WithRouteRemoved, WithIAMStatementRemoved):
// return a copy of ir with only the targeted node's own rule list changed.
//
// target_deregistration (the Card's third fault type) is implemented at the bottom of
// this file, over a fact the CANVAS producer already authors (a routes_to edge from a
// load balancer to its target). HCL ingest still has no such fact (golden/aws has no
// aws_lb_target_group_attachment), so for HCL bundles registration stays unknown and
// the fault is refused rather than guessed.
package core

import "reflect"

// mutateNodeRawRuleList is the shared copy-on-write primitive for both rule-list fault
// types below: find nodeID, apply mutate to a COPY of its RawAttributes[key] rule
// list, leaving every other node (and the original ir) untouched.
func mutateNodeRawRuleList(ir *IR, nodeID, key string, mutate func([]map[string]any) ([]map[string]any, bool)) (*IR, bool) {
	nodes := make([]Node, len(ir.Nodes))
	copy(nodes, ir.Nodes)

	for i, n := range nodes {
		if n.ID != nodeID {
			continue
		}
		raw := rawRuleMaps(n.RawAttributes[key])
		mutated, ok := mutate(raw)
		if !ok {
			return ir, false
		}
		rawAttrs := make(map[string]any, len(n.RawAttributes)+1)
		for k, v := range n.RawAttributes {
			rawAttrs[k] = v
		}
		rawAttrs[key] = mutated
		n.RawAttributes = rawAttrs
		nodes[i] = n
		return &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: nodes, Edges: ir.Edges}, true
	}
	return ir, false
}

// WithSGRuleRemoved returns a copy of ir with the one rule on sgID exactly matching
// rule removed. ok is false, and ir is returned unchanged, when sgID does not exist
// or has no rule exactly matching rule — refusing to guess which rule was meant,
// same discipline as WithIAMStatementRemoved's own Sid match.
func WithSGRuleRemoved(ir *IR, sgID string, rule SGRule) (*IR, bool) {
	return mutateNodeRawRuleList(ir, sgID, "security_group_rules", func(raw []map[string]any) ([]map[string]any, bool) {
		idx := -1
		for i, r := range raw {
			if reflect.DeepEqual(toSGRule(r), rule) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil, false
		}
		out := append(append([]map[string]any{}, raw[:idx]...), raw[idx+1:]...)
		return out, true
	})
}

// WithSGRuleAdded returns a copy of ir with rule appended (verbatim) to sgID's own
// rule list.
func WithSGRuleAdded(ir *IR, sgID string, rule SGRule) (*IR, bool) {
	return mutateNodeRawRuleList(ir, sgID, "security_group_rules", func(raw []map[string]any) ([]map[string]any, bool) {
		return append(append([]map[string]any{}, raw...), fromSGRule(rule)), true
	})
}

// WithNACLRuleRemoved is WithSGRuleRemoved's own NACL equivalent.
func WithNACLRuleRemoved(ir *IR, naclID string, rule NACLRule) (*IR, bool) {
	return mutateNodeRawRuleList(ir, naclID, "nacl_rules", func(raw []map[string]any) ([]map[string]any, bool) {
		idx := -1
		for i, r := range raw {
			if reflect.DeepEqual(toNACLRule(r), rule) {
				idx = i
				break
			}
		}
		if idx == -1 {
			return nil, false
		}
		out := append(append([]map[string]any{}, raw[:idx]...), raw[idx+1:]...)
		return out, true
	})
}

// WithNACLRuleAdded is WithSGRuleAdded's own NACL equivalent.
func WithNACLRuleAdded(ir *IR, naclID string, rule NACLRule) (*IR, bool) {
	return mutateNodeRawRuleList(ir, naclID, "nacl_rules", func(raw []map[string]any) ([]map[string]any, bool) {
		return append(append([]map[string]any{}, raw...), fromNACLRule(rule)), true
	})
}

// fromSGRule/fromNACLRule are toSGRule/toNACLRule's own inverses (core/
// securitygroups.go, core/nacl.go) — building the same raw attribute shape
// ingest/securitygroups.go and ingest/nacl.go themselves produce, so a mutated node
// is indistinguishable from one ingest built this way natively.
func fromSGRule(r SGRule) map[string]any {
	m := map[string]any{"direction": r.Direction, "protocol": r.Protocol, "from_port": r.FromPort, "to_port": r.ToPort}
	if len(r.CIDRs) > 0 {
		cidrs := make([]any, len(r.CIDRs))
		for i, c := range r.CIDRs {
			cidrs[i] = c
		}
		m["cidr_blocks"] = cidrs
	}
	if r.SourceSG != "" {
		m["source_security_group"] = r.SourceSG
	}
	return m
}

func fromNACLRule(r NACLRule) map[string]any {
	return map[string]any{
		"number": r.Number, "direction": r.Direction, "protocol": r.Protocol,
		"cidr": r.CIDR, "allow": r.Allow, "from_port": r.FromPort, "to_port": r.ToPort,
	}
}

// lbTargetEdges returns the routes_to edges that mean "this load balancer serves
// traffic to that target" — a routes_to edge OUT of a load_balancer node with no
// destination_cidr (route-table routes always carry one). The canvas authors these
// explicitly; HCL ingest derives them from aws_lb_target_group_attachment (PC-150) and
// marks them attachments-only. A load balancer with none (golden/aws registers its
// targets through a controller) has registration UNKNOWN — never assumed either way (I4).
func lbTargetEdges(ir *IR, lbID string) []Edge {
	var out []Edge
	for _, e := range ir.Edges {
		if e.Type != EdgeTypeRoutesTo || e.From != lbID {
			continue
		}
		if cidr, _ := e.RawAttributes["destination_cidr"].(string); cidr != "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

// WithTargetDeregistered is PC-130's target_deregistration: a COPY of ir in which the
// one LB→target registration is retargeted to a sentinel (the same device
// WithRouteRemoved uses), so the load balancer still HAS registration facts — it
// simply no longer lists targetID. ok is false, ir unchanged, when lbID is not a
// load_balancer or has no such registration to remove: refusing to guess.
func WithTargetDeregistered(ir *IR, lbID, targetID string) (*IR, bool) {
	n, found := nodeByID(ir, lbID)
	if !found || n.Type != NodeTypeLoadBalancer {
		return ir, false
	}
	out := &IR{SchemaVersion: ir.SchemaVersion, VersionNumber: ir.VersionNumber, VersionHash: ir.VersionHash, Nodes: ir.Nodes}
	changed := false
	for _, e := range ir.Edges {
		if !changed && e.Type == EdgeTypeRoutesTo && e.From == lbID && e.To == targetID {
			if cidr, _ := e.RawAttributes["destination_cidr"].(string); cidr == "" {
				e.To = removedRouteTargetPrefix + e.To
				changed = true
			}
		}
		out.Edges = append(out.Edges, e)
	}
	if !changed {
		return ir, false
	}
	return out, true
}

// registrationState is what the IR says about whether a destination is registered with a load balancer.
type registrationState int

const (
	// registrationUnmodelled: the IR carries no registration facts for this load balancer at all
	// (an HCL bundle with no attachment resources, or controller-managed targets). No claim.
	registrationUnmodelled registrationState = iota
	registrationRegistered
	// registrationDeregistered: the destination WAS registered and a deregistration fault removed it.
	// The only way an attachments-only load balancer yields a deny.
	registrationDeregistered
	// registrationNotListed: registration facts exist, the destination is not among them, and they are
	// attachments-only (PC-150), so the destination may still be registered by an Auto Scaling group, an
	// ECS service or a controller. Not assessable, never a deny.
	registrationNotListed
	// registrationAbsent: complete registration facts (canvas-authored) and the destination is not in them.
	registrationAbsent
)

// registrationAttachmentsOnly is the value ingest stamps as RawAttributes["registration_scope"] on an
// edge derived only from HCL attachment resources (ingest.RegistrationScopeAttachments).
const registrationAttachmentsOnly = "attachments"

// targetRegistration reports, for a hop whose source is a load balancer, what the IR says about
// whether destID is registered.
func targetRegistration(ir *IR, lbID, destID string) registrationState {
	edges := lbTargetEdges(ir, lbID)
	if len(edges) == 0 {
		return registrationUnmodelled
	}
	for _, e := range edges {
		if e.To == destID {
			return registrationRegistered
		}
	}
	for _, e := range edges {
		if e.To == removedRouteTargetPrefix+destID {
			return registrationDeregistered
		}
	}
	for _, e := range edges {
		if e.RawAttributes["registration_scope"] != registrationAttachmentsOnly {
			return registrationAbsent
		}
	}
	return registrationNotListed
}
