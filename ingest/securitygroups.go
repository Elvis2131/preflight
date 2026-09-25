// This file is PC-112's ingest-side half: real Terraform Security Group rule data
// (both aws_security_group's own inline ingress{}/egress{} blocks, repeated correctly
// via ParsedResource.NestedBlocks — PC-111's own mechanism, reused here — and the
// standalone aws_security_group_rule resource) merged into the owning SG node's own
// RawAttributes, as a list of normalized rule maps under "security_group_rules". No IR
// schema change needed: Node.RawAttributes is already untyped (map[string]any),
// exactly the same "real per-element data a fixed field set can't anticipate" fit
// Edge.RawAttributes (PC-111) formalized for edges. core/securitygroups.go reads this
// same shape back out into core/internal/analyse's real SGRule/SGProfile types.
package ingest

import "preflight/core"

// sgRuleAttrs are the real Terraform attribute names, identical across both real
// shapes (an inline ingress/egress block and a standalone aws_security_group_rule),
// that this function reads.
const (
	sgAttrProtocol   = "protocol"
	sgAttrFromPort   = "from_port"
	sgAttrToPort     = "to_port"
	sgAttrCIDRBlocks = "cidr_blocks"
	sgAttrSourceSGs  = "security_groups"          // inline block's own list-of-refs attribute
	sgAttrSourceSG1  = "source_security_group_id" // aws_security_group_rule's own single-ref attribute
)

// normalizeSGRule builds one real rule map from a set of literal attributes plus any
// resolved resource references found alongside them (both shapes' own real attribute
// names, above) — shared by both the inline-block and standalone-resource paths so
// there is exactly one place that decides what a "rule" looks like.
func normalizeSGRule(direction string, attrs map[string]any, refs map[string]ResourceRef) map[string]any {
	rule := map[string]any{"direction": direction}
	if v, ok := attrs[sgAttrProtocol]; ok {
		rule["protocol"] = v
	}
	if v, ok := attrs[sgAttrFromPort]; ok {
		rule["from_port"] = v
	}
	if v, ok := attrs[sgAttrToPort]; ok {
		rule["to_port"] = v
	}
	if v, ok := attrs[sgAttrCIDRBlocks]; ok {
		rule["cidr_blocks"] = v
	}
	if ref, ok := refs[sgAttrSourceSGs]; ok {
		rule["source_security_group"] = ref.Key()
	} else if ref, ok := refs[sgAttrSourceSG1]; ok {
		rule["source_security_group"] = ref.Key()
	}
	return rule
}

// mergeSecurityGroupRules walks every parsed resource and attaches real SG rule data
// onto each aws_security_group node's RawAttributes["security_group_rules"] — called
// after nodes are built (buildNode), mutating the already-built node slice in place by
// ID, the same "post-process onto already-built nodes" shape buildRouteEdges uses for
// routes elsewhere in this package.
func mergeSecurityGroupRules(nodes []core.Node, parsed []ParsedResource) {
	rulesBySG := map[string][]map[string]any{}

	for _, r := range parsed {
		if r.Type == "aws_security_group" {
			for _, direction := range []string{"ingress", "egress"} {
				for _, nb := range r.NestedBlocks[direction] {
					rulesBySG[r.Key()] = append(rulesBySG[r.Key()], normalizeSGRule(direction, nb.Attributes, nb.References))
				}
			}
			continue
		}
		if r.Type == "aws_security_group_rule" {
			owner, ok := r.AttributeReferences["security_group_id"]
			if !ok || len(owner) == 0 {
				continue // no resolvable owner — nothing to attach this rule to
			}
			direction, _ := r.Attributes["type"].(string) // real attribute: "ingress" | "egress"
			if direction == "" {
				continue
			}
			refs := map[string]ResourceRef{}
			for name, rs := range r.AttributeReferences {
				if len(rs) > 0 {
					refs[name] = rs[0]
				}
			}
			rulesBySG[owner[0].Key()] = append(rulesBySG[owner[0].Key()], normalizeSGRule(direction, r.Attributes, refs))
		}
	}

	for i := range nodes {
		if rules, ok := rulesBySG[nodes[i].ID]; ok {
			if nodes[i].RawAttributes == nil {
				nodes[i].RawAttributes = map[string]any{}
			}
			nodes[i].RawAttributes["security_group_rules"] = rules
		}
	}
}
