// This file is PC-158: ingest never silently drops a declared value it cannot read. Where the engine
// reads an attribute (providers/*/ unresolved_matters), a rule inside a security group or route table, or
// an association, a declaration ingest cannot read is recorded on the node concerned as
// RawAttributes["unresolved_inputs"] (see core/unresolved.go), and the result that depends on it is
// not_assessable. Before this an unreadable value looked exactly like an absent one, and PC-157 found the
// consequence: a policy written as jsonencode() was dropped and the role "satisfied" least privilege.
package ingest

import (
	"fmt"
	"sort"

	"preflight/core"
	"preflight/providers"
)

type unresolvedStamper struct {
	edges []core.Edge // unresolved edges to add (a route table keeps its identity when a route is unreadable)
	nodes []core.Node
	byID  map[string]int
	byKey map[string]ParsedResource
	reg   providers.Registry
}

func newUnresolvedStamper(nodes []core.Node, byKey map[string]ParsedResource, reg providers.Registry) *unresolvedStamper {
	s := &unresolvedStamper{nodes: nodes, byID: map[string]int{}, byKey: byKey, reg: reg}
	for i, n := range nodes {
		s.byID[n.ID] = i
	}
	return s
}

// add records "<affects>: <detail>" on the node with that ID.
func (s *unresolvedStamper) add(nodeID, affects, detail string) {
	i, ok := s.byID[nodeID]
	if !ok {
		return
	}
	if s.nodes[i].RawAttributes == nil {
		s.nodes[i].RawAttributes = map[string]any{}
	}
	cur, _ := s.nodes[i].RawAttributes[core.UnresolvedInputsAttr].([]string)
	entry := affects + ": " + detail
	for _, c := range cur {
		if c == entry {
			return
		}
	}
	cur = append(cur, entry)
	sort.Strings(cur)
	s.nodes[i].RawAttributes[core.UnresolvedInputsAttr] = cur
}

// addAll records the entry on every node accepted by pick.
func (s *unresolvedStamper) addAll(pick func(core.Node) bool, affects, detail string) {
	for _, n := range s.nodes {
		if pick(n) {
			s.add(n.ID, affects, detail)
		}
	}
}

// relationshipProblem says why info cannot serve as a relationship to a modelled resource, or "".
// A relationship must be nothing but references to resources that become nodes; a literal id, a variable
// or any expression names something the engine cannot see.
func (s *unresolvedStamper) relationshipProblem(info AttrInfo) string {
	switch info.Shape {
	case ShapeOther:
		return "its value is not a static reference (a variable, function or expression)"
	case ShapeLiteral:
		return "it names something outside this bundle (a literal value, not a reference to a resource in it)"
	}
	for _, ref := range info.Refs {
		if !nodeWillExist(ref, s.byKey, s.reg) {
			return "it references " + ref.Key() + ", which is not a modelled resource in this bundle"
		}
	}
	return ""
}

// stampMatters applies each mapping's unresolved_matters to its resources.
func (s *unresolvedStamper) stampMatters(parsed []ParsedResource) {
	for _, r := range parsed {
		mapping, ok := s.reg.Lookup(r.Type)
		if !ok || mapping.IsEdgeMapping() {
			continue
		}
		for _, m := range mapping.UnresolvedMatters {
			info, declared := r.AttrInfo[m.Attribute]
			if !declared {
				continue // genuinely absent: a documented state, handled where the attribute is read
			}
			var problem string
			if m.Value {
				if info.Shape == ShapeOther {
					problem = "its value is not a static value (a variable, function or expression)"
				}
			} else {
				problem = s.relationshipProblem(info)
			}
			if problem != "" {
				s.add(r.Key(), m.Affects, m.Attribute+": "+problem)
			}
		}
	}
}

// stampAssociations records, on every subnet, an association resource that failed to produce its edge.
// An association whose endpoint could not be read could concern any subnet, so none may be read as
// "not associated".
func (s *unresolvedStamper) stampAssociations(parsed []ParsedResource, edgeOnly []EdgeOnlyResource) {
	produced := map[string]bool{}
	for _, e := range edgeOnly {
		produced[e.ResourceType+"."+e.ResourceName] = e.Produced
	}
	for _, r := range parsed {
		mapping, ok := s.reg.Lookup(r.Type)
		if !ok || !mapping.IsEdgeMapping() || mapping.Edge.Affects == "" {
			continue
		}
		if produced[r.Key()] {
			continue
		}
		s.addAll(func(n core.Node) bool { return n.RawAttributes["network_role"] == "subnet" },
			mapping.Edge.Affects, r.Key()+": an association whose subnet or target could not be read")
	}
}

var sgRuleValueFields = []string{"protocol", "from_port", "to_port", "cidr_blocks"}

// sgRuleUnmodelledFields name rule sources the engine does not model; a rule using one is not silently
// reduced to "matches nothing".
var sgRuleUnmodelledFields = []string{"ipv6_cidr_blocks", "prefix_list_ids", "self"}

// sgRuleProblems lists why one security-group rule (an inline block or a standalone resource) cannot be
// read in full. info maps the rule's attribute names to what ingest knows.
func (s *unresolvedStamper) sgRuleProblems(info map[string]AttrInfo) []string {
	var out []string
	for _, f := range sgRuleValueFields {
		if i, ok := info[f]; ok && i.Shape != ShapeLiteral {
			out = append(out, f+": its value is not a static value")
		}
	}
	for _, f := range sgRuleUnmodelledFields {
		if _, ok := info[f]; ok {
			out = append(out, f+": this rule source is not modelled")
		}
	}
	for _, f := range []string{"security_groups", "source_security_group_id"} {
		if i, ok := info[f]; ok {
			if p := s.relationshipProblem(i); p != "" {
				out = append(out, f+": "+p)
			}
		}
	}
	return out
}

func (s *unresolvedStamper) stampSecurityGroupRules(parsed []ParsedResource) {
	var sgKeys []string
	for _, r := range parsed {
		if r.Type == "aws_security_group" {
			sgKeys = append(sgKeys, r.Key())
		}
	}
	for _, r := range parsed {
		switch r.Type {
		case "aws_security_group":
			if r.HasDynamicBlock {
				s.add(r.Key(), core.AffectsSGRules, "a dynamic block generates rules ingest cannot expand")
			}
			for _, dir := range []string{"ingress", "egress"} {
				for i, nb := range r.NestedBlocks[dir] {
					for _, p := range s.sgRuleProblems(nb.Info) {
						s.add(r.Key(), core.AffectsSGRules, fmt.Sprintf("%s[%d].%s", dir, i, p))
					}
				}
			}
		case "aws_security_group_rule":
			owner := r.AttrInfo["security_group_id"]
			ownerProblem := s.relationshipProblem(owner)
			var problems []string
			if t, ok := r.AttrInfo["type"]; !ok || t.Shape != ShapeLiteral {
				problems = append(problems, "type: the rule direction is not a static value")
			}
			problems = append(problems, s.sgRuleProblems(r.AttrInfo)...)
			switch {
			case ownerProblem != "" || len(owner.Refs) != 1:
				// The rule cannot be attached to any group, so it could belong to any of them.
				for _, k := range sgKeys {
					s.add(k, core.AffectsSGRules, r.Key()+": security_group_id: the owning group could not be read")
				}
			default:
				for _, p := range problems {
					s.add(owner.Refs[0].Key(), core.AffectsSGRules, r.Key()+"."+p)
				}
			}
		}
	}
}

var naclUnmodelledFields = []string{"ipv6_cidr_block", "icmp_type", "icmp_code"}

// naclRuleProblems lists why one NACL rule (inline block or standalone resource) cannot be read in full.
func naclRuleProblems(info map[string]AttrInfo, numberAttr, actionAttr string) []string {
	var out []string
	for _, f := range []string{numberAttr, actionAttr, "protocol", "from_port", "to_port", "cidr_block"} {
		if i, ok := info[f]; ok && i.Shape != ShapeLiteral {
			out = append(out, f+": its value is not a static value")
		}
	}
	for _, f := range naclUnmodelledFields {
		if _, ok := info[f]; ok {
			out = append(out, f+": this rule field is not modelled")
		}
	}
	return out
}

func (s *unresolvedStamper) stampNACLs(parsed []ParsedResource) {
	var naclKeys []string
	for _, r := range parsed {
		if r.Type == "aws_network_acl" || r.Type == "aws_default_network_acl" {
			naclKeys = append(naclKeys, r.Key())
		}
	}
	isSubnet := func(n core.Node) bool { return n.RawAttributes["network_role"] == "subnet" }
	for _, r := range parsed {
		switch r.Type {
		case "aws_network_acl", "aws_default_network_acl":
			if r.HasDynamicBlock {
				s.add(r.Key(), core.AffectsNACL, "a dynamic block generates rules ingest cannot expand")
			}
			for _, dir := range []string{"ingress", "egress"} {
				for i, nb := range r.NestedBlocks[dir] {
					for _, p := range naclRuleProblems(nb.Info, naclAttrInlineRuleNo, naclAttrInlineAction) {
						s.add(r.Key(), core.AffectsNACL, fmt.Sprintf("%s[%d].%s", dir, i, p))
					}
				}
			}
			// Inline subnet associations: refs become edges (buildNACLAssociationEdges); anything else is an
			// association ingest cannot place, which could concern any subnet.
			if info, ok := r.AttrInfo["subnet_ids"]; ok && r.Type == "aws_network_acl" {
				if p := s.relationshipProblem(info); p != "" {
					s.addAll(isSubnet, core.AffectsNACL, r.Key()+".subnet_ids: "+p)
				}
			}
		case "aws_network_acl_rule":
			owner := r.AttrInfo["network_acl_id"]
			var problems []string
			if e, ok := r.AttrInfo[naclAttrEgress]; ok && e.Shape != ShapeLiteral {
				problems = append(problems, "egress: the rule direction is not a static value")
			}
			problems = append(problems, naclRuleProblems(r.AttrInfo, naclAttrRuleNumber, naclAttrRuleAction)...)
			if s.relationshipProblem(owner) != "" || len(owner.Refs) != 1 {
				for _, k := range naclKeys {
					s.add(k, core.AffectsNACL, r.Key()+": network_acl_id: the owning network ACL could not be read")
				}
				continue
			}
			for _, p := range problems {
				s.add(owner.Refs[0].Key(), core.AffectsNACL, r.Key()+"."+p)
			}
		}
	}
}

// stampResourcePolicies records an aws_s3_bucket_policy ingest could not read, on its bucket. A resource-based
// policy can hold an explicit Deny, so dropping it silently turns a denied request into an allowed one.
func (s *unresolvedStamper) stampResourcePolicies(parsed []ParsedResource) {
	var bucketKeys []string
	for _, r := range parsed {
		if r.Type == "aws_s3_bucket" {
			bucketKeys = append(bucketKeys, r.Key())
		}
	}
	for _, r := range parsed {
		if r.Type != "aws_s3_bucket_policy" {
			continue
		}
		problem := ""
		if _, readable, why := readPolicyAttribute(r, "policy", r.Key(), s.byKey); !readable {
			problem = why
		}
		owner := r.AttrInfo["bucket"]
		if s.relationshipProblem(owner) != "" || len(owner.Refs) != 1 {
			// It cannot be attached to any bucket, so it may belong to any of them.
			for _, k := range bucketKeys {
				s.add(k, core.AffectsResourcePolicy, r.Key()+": bucket: the policy's bucket could not be read")
			}
			continue
		}
		if problem != "" {
			s.add(owner.Refs[0].Key(), core.AffectsResourcePolicy, r.Key()+": "+problem)
		}
	}
}

// routeTargetAttrs are the attributes that name a route's target.
var routeTargetAttrs = []string{"gateway_id", "nat_gateway_id", "transit_gateway_id", "vpc_endpoint_id",
	"vpc_peering_connection_id", "network_interface_id", "egress_only_gateway_id", "carrier_gateway_id", "local_gateway_id"}

func routeProblems(info map[string]AttrInfo, cidrAttr string) []string {
	var out []string
	if i, ok := info[cidrAttr]; ok && i.Shape != ShapeLiteral {
		out = append(out, cidrAttr+": the destination is not a static value")
	}
	for _, f := range routeTargetAttrs {
		if i, ok := info[f]; ok && i.Shape == ShapeOther {
			out = append(out, f+": the target is not a static reference")
		}
	}
	return out
}

// unresolvedRoute records an unreadable route on a table and keeps the table recognisable as one: the
// engine identifies a route table by its outgoing routes_to edges, so a table whose only route is
// unreadable would otherwise stop being a route table and silently drop out of subnet resolution.
func (s *unresolvedStamper) unresolvedRoute(tableKey, detail string) {
	s.add(tableKey, core.AffectsRoutes, detail)
	s.edges = append(s.edges, core.Edge{
		ID:         tableKey + "-route-unresolved[" + detail + "]",
		Type:       core.EdgeTypeRoutesTo,
		From:       tableKey,
		To:         "unresolved:" + tableKey + ":" + detail,
		Resolution: core.ResolutionUnresolved,
		Provenance: core.NewProvenance(core.KindStated, "ingest/unresolved").WithReason("a route could not be read: " + detail),
	})
}

func (s *unresolvedStamper) stampRoutes(parsed []ParsedResource) {
	var tableKeys []string
	for _, r := range parsed {
		if r.Type == "aws_route_table" || r.Type == "aws_default_route_table" {
			tableKeys = append(tableKeys, r.Key())
		}
	}
	for _, r := range parsed {
		switch r.Type {
		case "aws_route_table", "aws_default_route_table":
			if r.HasDynamicBlock {
				s.unresolvedRoute(r.Key(), "a dynamic block generates routes ingest cannot expand")
			}
			for i, nb := range r.NestedBlocks["route"] {
				for _, p := range routeProblems(nb.Info, inlineRouteCIDRAttr) {
					s.unresolvedRoute(r.Key(), fmt.Sprintf("route[%d].%s", i, p))
				}
			}
		case "aws_route":
			owner := r.AttrInfo["route_table_id"]
			if s.relationshipProblem(owner) != "" || len(owner.Refs) != 1 {
				for _, k := range tableKeys {
					s.unresolvedRoute(k, r.Key()+": route_table_id: the owning route table could not be read")
				}
				continue
			}
			for _, p := range routeProblems(r.AttrInfo, standaloneRouteCIDRAttr) {
				s.unresolvedRoute(owner.Refs[0].Key(), r.Key()+"."+p)
			}
		}
	}
}

// stampUnresolvedInputs runs every PC-158 pass over the built nodes.
func stampUnresolvedInputs(nodes []core.Node, parsed []ParsedResource, byKey map[string]ParsedResource, reg providers.Registry, edgeOnly []EdgeOnlyResource) []core.Edge {
	s := newUnresolvedStamper(nodes, byKey, reg)
	s.stampMatters(parsed)
	s.stampAssociations(parsed, edgeOnly)
	s.stampSecurityGroupRules(parsed)
	s.stampRoutes(parsed)
	s.stampNACLs(parsed)
	s.stampResourcePolicies(parsed)
	return s.edges
}
