package core

// PC-105: server-side placement validation for canvas-authored VPC/subnet
// containers. The browser may highlight a violation, but THIS decides (the Card's own
// rule) — and it is pure (I1): a function of the posted CanvasDocument only.
//
// Only rules verified against AWS's own documentation are implemented; each one's
// Source is the page it was verified against, with the sentence it rests on. A rule
// that could not be verified is deliberately absent rather than guessed (CLAUDE.md §5)
// — in particular there is NO "EC2 instance must be in a subnet" rule: the EC2 user
// guide page consulted for it contained no citable sentence, so it is not asserted.
//
// Incompleteness is never a violation (I4): an Availability Zone that has not been
// declared yet makes the AZ-count rules inapplicable, not failed.

import (
	"fmt"
	"sort"
)

// Placement rule IDs — also the conformance tests' IDs' stems.
const (
	PlaceSubnetOneVPC           = "PLACE-SUBNET-ONE-VPC"
	PlaceALBTwoAZs              = "PLACE-ALB-TWO-AZS"
	PlaceDBSubnetGroupTwoAZs    = "PLACE-DBSUBNETGROUP-TWO-AZS"
	PlaceNATGatewayOneSubnet    = "PLACE-NAT-ONE-SUBNET"
	PlaceDefaultNACLOneVPC      = "PLACE-DEFAULT-NACL-ONE-VPC"
	PlaceDefaultNACLOnePerVPC   = "PLACE-DEFAULT-NACL-ONE-PER-VPC"
	serviceIDDefaultNACL        = "aws_default_network_acl"
	serviceIDVPC                = "aws_vpc"
	serviceIDSubnet             = "aws_subnet"
	serviceIDLB                 = "aws_lb"
	serviceIDDBSubnetGroup      = "aws_db_subnet_group"
	serviceIDNATGateway         = "aws_nat_gateway"
	sizingLoadBalancerType      = "load_balancer_type"
	loadBalancerTypeApplication = "application"
)

// PlacementViolation names the rule broken, the resource that breaks it, and the AWS
// documentation the rule comes from.
type PlacementViolation struct {
	Rule       string `json:"rule"`
	ResourceID string `json:"resource_id"`
	Message    string `json:"message"`
	Source     string `json:"source"`
}

// String renders one violation for an error message: rule, resource, why.
func (v PlacementViolation) String() string {
	return fmt.Sprintf("[%s] %s: %s", v.Rule, v.ResourceID, v.Message)
}

// ValidateCanvasPlacement returns every placement violation in doc, sorted by rule
// then resource ID (NFR-1), or nil. Nesting is the contained_in edge (child → parent):
// Region ⊃ VPC ⊃ AZ ⊃ Subnet ⊃ resource is expressed as resource → subnet → VPC with
// the AZ as the subnet's own availability_zone attribute (see CanvasNode).
func ValidateCanvasPlacement(doc CanvasDocument) []PlacementViolation {
	byID := make(map[string]CanvasNode, len(doc.Nodes))
	for _, n := range doc.Nodes {
		byID[n.ID] = n
	}
	// parents[x] = IDs x is contained_in, restricted to nodes that exist (a dangling
	// edge is the ingest path's own unresolved-edge case, not a placement fact).
	parents := map[string][]string{}
	for _, e := range doc.Edges {
		if e.Type != EdgeTypeContainedIn {
			continue
		}
		if _, ok := byID[e.From]; !ok {
			continue
		}
		if _, ok := byID[e.To]; !ok {
			continue
		}
		parents[e.From] = append(parents[e.From], e.To)
	}
	parentsOfService := func(id, serviceID string) []CanvasNode {
		var out []CanvasNode
		for _, pid := range parents[id] {
			if p := byID[pid]; p.ServiceID == serviceID {
				out = append(out, p)
			}
		}
		return out
	}

	var out []PlacementViolation
	add := func(rule, id, msg, src string) {
		out = append(out, PlacementViolation{Rule: rule, ResourceID: id, Message: msg, Source: src})
	}

	for _, n := range doc.Nodes {
		switch n.ServiceID {
		case serviceIDSubnet:
			// A subnet belongs to exactly one VPC.
			if vpcs := parentsOfService(n.ID, serviceIDVPC); len(vpcs) != 1 {
				add(PlaceSubnetOneVPC, n.ID,
					fmt.Sprintf("a subnet must be contained in exactly one VPC, but this one is in %d", len(vpcs)),
					"https://docs.aws.amazon.com/vpc/latest/userguide/configure-subnets.html")
			}

		case serviceIDDefaultNACL:
			// PC-153: a default network ACL is a VPC's own ("Your virtual private cloud (VPC)
			// automatically comes with a default network ACL"), so it is contained in exactly
			// one VPC; the engine uses it for that VPC's subnets with no explicit association.
			if vpcs := parentsOfService(n.ID, serviceIDVPC); len(vpcs) != 1 {
				add(PlaceDefaultNACLOneVPC, n.ID,
					fmt.Sprintf("a default network ACL belongs to exactly one VPC, but this one is in %d", len(vpcs)),
					"https://docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html")
			}

		case serviceIDNATGateway:
			// A (zonal) NAT gateway is created in exactly one subnet.
			if subs := parentsOfService(n.ID, serviceIDSubnet); len(subs) != 1 {
				add(PlaceNATGatewayOneSubnet, n.ID,
					fmt.Sprintf("a NAT gateway is created in exactly one subnet, but this one is in %d", len(subs)),
					"https://docs.aws.amazon.com/vpc/latest/userguide/nat-gateway-working-with.html")
			}

		case serviceIDLB:
			// Only an Application Load Balancer's rule is verified; any other (or
			// unstated) type is out of this rule's scope, never assumed to be one.
			if n.Sizing[sizingLoadBalancerType] != loadBalancerTypeApplication {
				continue
			}
			if msg := needTwoAZs(parentsOfService(n.ID, serviceIDSubnet), "an Application Load Balancer"); msg != "" {
				add(PlaceALBTwoAZs, n.ID, msg,
					"https://docs.aws.amazon.com/elasticloadbalancing/latest/application/application-load-balancers.html")
			}

		case serviceIDDBSubnetGroup:
			if msg := needTwoAZs(parentsOfService(n.ID, serviceIDSubnet), "a DB subnet group"); msg != "" {
				add(PlaceDBSubnetGroupTwoAZs, n.ID, msg,
					"https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_VPC.WorkingWithRDSInstanceinaVPC.html")
			}
		}
	}

	// A VPC has one default network ACL: two declared for the same VPC leave the engine no
	// way to know which rules the VPC's unassociated subnets use, so each extra one is named.
	defaultsByVPC := map[string][]string{}
	for _, n := range doc.Nodes {
		if n.ServiceID != serviceIDDefaultNACL {
			continue
		}
		for _, vpc := range parentsOfService(n.ID, serviceIDVPC) {
			defaultsByVPC[vpc.ID] = append(defaultsByVPC[vpc.ID], n.ID)
		}
	}
	for vpcID, ids := range defaultsByVPC {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		for _, id := range ids {
			add(PlaceDefaultNACLOnePerVPC, id,
				fmt.Sprintf("a VPC has one default network ACL, but %s has %d (%v)", vpcID, len(ids), ids),
				"https://docs.aws.amazon.com/vpc/latest/userguide/default-network-acl.html")
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out
}

// needTwoAZs reports why subnets do not cover two distinct Availability Zones, or ""
// if they do, or if the answer is not yet knowable. Fewer than two subnets is a
// violation regardless of AZ values; with two or more, the rule is decided only when
// EVERY subnet's availability_zone is declared (an undeclared one might supply the
// second zone, so claiming a violation would be a guess).
func needTwoAZs(subnets []CanvasNode, what string) string {
	if len(subnets) < 2 {
		return fmt.Sprintf("%s requires subnets in at least two Availability Zones, but this one is in %d subnet(s)", what, len(subnets))
	}
	zones := map[string]bool{}
	for _, s := range subnets {
		if s.AvailabilityZone == "" {
			return "" // undeclared AZ: not assessable yet, not a violation (I4)
		}
		zones[s.AvailabilityZone] = true
	}
	if len(zones) < 2 {
		return fmt.Sprintf("%s requires subnets in at least two Availability Zones, but all %d of this one's subnets are in a single zone", what, len(subnets))
	}
	return ""
}
