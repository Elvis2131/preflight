// This file is PC-158's shared vocabulary for "a value the engine depends on was declared but could not
// be read". PC-157 found that a policy written as jsonencode() was dropped silently and the engine then
// reported a vacuous "satisfied". An unreadable value and an absent one look identical downstream, and
// absent often means a documented default, so ingest records every declared-but-unreadable input the
// engine reads, and the engine turns the result that depends on it into not_assessable (I4).
//
// Ingest stamps RawAttributes[UnresolvedInputsAttr] on the node concerned: a sorted list of
// "<affects>: <detail>". The "affects" prefix names the question the input bears on, so each consumer
// asks only about its own. Which attributes matter is data (providers/*/ unresolved_matters), not code.
package core

import "strings"

// UnresolvedInputsAttr is the RawAttributes key ingest writes.
const UnresolvedInputsAttr = "unresolved_inputs"

// What an unreadable input affects.
const (
	AffectsPlacement      = "placement"       // which subnet / VPC / zone the node lives in
	AffectsSecurityGroups = "security_groups" // which security groups are attached
	AffectsSGRules        = "sg_rules"        // the rules inside a security group
	AffectsRouting        = "routing"         // which route table a subnet uses
	AffectsRoutes         = "routes"          // the routes inside a route table
	AffectsNACL           = "nacl"            // which network ACL a subnet uses, or its rules
	AffectsTierLabel      = "tier_label"      // the tag a check reads to learn a subnet's declared tier
	AffectsExposure       = "exposure"        // whether a load balancer is internet-facing
	AffectsCost           = "cost"            // a value the price lookup selects a rate by
	AffectsResourcePolicy = "resource_policy" // the resource-based policy attached to a resource
)

// UnresolvedInputs returns n's unreadable inputs that bear on affects, with the prefix removed.
func UnresolvedInputs(n Node, affects string) []string {
	var out []string
	for _, s := range stringList(n.RawAttributes[UnresolvedInputsAttr]) {
		if rest, ok := strings.CutPrefix(s, affects+": "); ok {
			out = append(out, rest)
		}
	}
	return out
}

// anyUnresolved collects every node's unreadable inputs for one concern, each prefixed by the node ID,
// in node order (the IR's nodes are sorted, so this is deterministic).
func anyUnresolved(ir *IR, affects string) []string {
	var out []string
	for _, n := range ir.Nodes {
		for _, u := range UnresolvedInputs(n, affects) {
			out = append(out, n.ID+": "+u)
		}
	}
	return out
}
