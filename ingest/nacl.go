// This file is PC-113's ingest-side half: real Terraform Network ACL rule data (both
// aws_network_acl's own inline ingress{}/egress{} blocks, and the standalone
// aws_network_acl_rule resource) merged into the owning NACL node's own
// RawAttributes, mirroring ingest/securitygroups.go's own approach exactly — same
// reasoning, same mechanism (ParsedResource.NestedBlocks, PC-111's own addition), no
// IR schema change needed.
package ingest

import "preflight/core"

// naclRuleAttrs are the real Terraform attribute names. aws_network_acl_rule's own
// "egress" is a bool (true=egress, false=ingress); its inline block counterpart is
// named by the block type itself ("ingress"/"egress"), matching security groups'
// own shape.
const (
	naclAttrRuleNumber = "rule_number"
	naclAttrProtocol   = "protocol"
	naclAttrFromPort   = "from_port"
	naclAttrToPort     = "to_port"
	naclAttrCIDRBlock  = "cidr_block"
	naclAttrRuleAction = "rule_action" // "allow" | "deny" — real Terraform attribute name, both shapes
	naclAttrEgress     = "egress"      // standalone aws_network_acl_rule's own bool attribute
)

func normalizeNACLRule(direction string, attrs map[string]any) map[string]any {
	rule := map[string]any{"direction": direction}
	if v, ok := attrs[naclAttrRuleNumber]; ok {
		rule["number"] = v
	}
	if v, ok := attrs[naclAttrProtocol]; ok {
		rule["protocol"] = v
	}
	if v, ok := attrs[naclAttrFromPort]; ok {
		rule["from_port"] = v
	}
	if v, ok := attrs[naclAttrToPort]; ok {
		rule["to_port"] = v
	}
	if v, ok := attrs[naclAttrCIDRBlock]; ok {
		rule["cidr"] = v
	}
	if action, ok := attrs[naclAttrRuleAction].(string); ok {
		rule["allow"] = action == "allow"
	}
	return rule
}

// mergeNetworkACLRules mirrors mergeSecurityGroupRules exactly — walks every parsed
// resource, attaches real NACL rule data onto each aws_network_acl node's
// RawAttributes["nacl_rules"].
func mergeNetworkACLRules(nodes []core.Node, parsed []ParsedResource) {
	rulesByNACL := map[string][]map[string]any{}

	for _, r := range parsed {
		if r.Type == "aws_network_acl" || r.Type == "aws_default_network_acl" {
			// A NACL with no rule blocks is still a NACL carrying ZERO rules (a custom
			// NACL denies everything; AWS VPC User Guide, "Custom network ACLs"), which
			// is different from "no NACL known" — so the key is always present.
			if _, ok := rulesByNACL[r.Key()]; !ok {
				rulesByNACL[r.Key()] = []map[string]any{}
			}
			for _, direction := range []string{"ingress", "egress"} {
				for _, nb := range r.NestedBlocks[direction] {
					rulesByNACL[r.Key()] = append(rulesByNACL[r.Key()], normalizeNACLRule(direction, nb.Attributes))
				}
			}
			continue
		}
		if r.Type == "aws_network_acl_rule" {
			owner, ok := r.AttributeReferences["network_acl_id"]
			if !ok || len(owner) == 0 {
				continue
			}
			direction := "ingress"
			if egress, _ := r.Attributes[naclAttrEgress].(bool); egress {
				direction = "egress"
			}
			rulesByNACL[owner[0].Key()] = append(rulesByNACL[owner[0].Key()], normalizeNACLRule(direction, r.Attributes))
		}
	}

	for i := range nodes {
		if rules, ok := rulesByNACL[nodes[i].ID]; ok {
			if nodes[i].RawAttributes == nil {
				nodes[i].RawAttributes = map[string]any{}
			}
			nodes[i].RawAttributes["nacl_rules"] = rules
		}
	}
	// PC-149: mark the VPC default NACL so core can find it for unassociated subnets.
	defaults := map[string]bool{}
	for _, r := range parsed {
		if r.Type == "aws_default_network_acl" {
			defaults[r.Key()] = true
		}
	}
	for i := range nodes {
		if defaults[nodes[i].ID] {
			if nodes[i].RawAttributes == nil {
				nodes[i].RawAttributes = map[string]any{}
			}
			nodes[i].RawAttributes["default_nacl"] = true
		}
	}
}
