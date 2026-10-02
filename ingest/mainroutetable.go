// This file is PC-151's ingest half: which route table is a VPC's MAIN one. Two Terraform
// shapes declare it — aws_default_route_table (adopts the VPC's own default table, which IS
// the main table) and aws_main_route_table_association (makes a named table the main one).
// Both stamp the route table node RawAttributes["main_route_table"]=true; core then uses
// that for a subnet with no explicit association (core.ResolveSubnetRouteTable).
package ingest

import "preflight/core"

func markMainRouteTables(nodes []core.Node, parsed []ParsedResource) {
	main := map[string]bool{}
	for _, r := range parsed {
		switch r.Type {
		case "aws_default_route_table":
			main[r.Key()] = true
		case "aws_main_route_table_association":
			if refs := r.AttributeReferences["route_table_id"]; len(refs) > 0 {
				main[refs[0].Key()] = true
			}
		}
	}
	for i := range nodes {
		if !main[nodes[i].ID] {
			continue
		}
		if nodes[i].RawAttributes == nil {
			nodes[i].RawAttributes = map[string]any{}
		}
		nodes[i].RawAttributes["main_route_table"] = true
	}
}
