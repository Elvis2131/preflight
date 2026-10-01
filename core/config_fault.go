// This file is PC-130: configuration-change faults — "the infrastructure is intact
// but a rule changed," a different failure class from PC-14/78/129's own "a thing
// disappears" faults, invisible to topology-only analysis. Two fault types are
// implemented here, both hand-verified against the real golden bundle:
// sg_rule_change and nacl_rule_change. Both reuse the SAME copy-on-write IR-mutation
// discipline PC-129/135 established (WithRouteRemoved, WithIAMStatementRemoved):
// return a copy of ir with only the targeted node's own rule list changed.
//
// target_deregistration (the Card's third named fault type — "an LB target group
// loses members") is NOT implemented here. Investigated before writing any code:
// golden/aws has a real aws_lb_target_group resource (alb.tf) but NO
// aws_lb_target_group_attachment and no ECS/EKS service load_balancer block wiring a
// real target into it anywhere in the bundle — the golden architecture's own real
// design (EKS + an implied Ingress/ALB-controller-managed target registration) has NO
// static-literal Terraform fact for "which resource is a registered target," which is
// exactly CLAUDE.md §15's ingest-scope boundary (dynamic/controller-managed state,
// not a static literal). There is also no existing IR edge type at all representing
// "this load balancer serves traffic to this specific target" (BuildTrace's own
// route/SG/NACL steps never consult target-group membership — network reachability
// and LB-to-backend registration are orthogonal facts in this IR today). Building
// that honestly would mean adding a real, separate piece of ingest+IR modelling (a
// new edge type, three new resource-type mappings, and a new BuildTrace step) with no
// real golden data to hand-verify it against — exactly the kind of scope expansion
// this ticket's own Card does not ask for and this session's discipline says not to
// invent. Left as a real, stated gap, not silently attempted against fabricated data.
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
