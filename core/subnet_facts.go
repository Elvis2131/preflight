package core

// PC-105 (amended by its decision comment): the workspace draws Region and AZ as READ-ONLY
// groupings and public/private as a BADGE — all of it DERIVED here from the model, never
// authored on the picture. The architect sets a subnet's availability_zone and its routes;
// the groupings and the badge follow. This file is the one place those derivations are
// decided, so the browser only ever draws what the server says.

import (
	"regexp"
	"sort"
)

// SubnetVisibility is a subnet's route-derived public/private classification.
type SubnetVisibility string

const (
	SubnetPublic  SubnetVisibility = "public"
	SubnetPrivate SubnetVisibility = "private"
	// SubnetVisibilityUnknown: the subnet has no resolvable route table, so whether it
	// has a route to an internet gateway cannot be said (I4) — never defaulted to private.
	SubnetVisibilityUnknown SubnetVisibility = "not_assessable"
)

// SubnetFact is everything the workspace draws about one subnet, derived from the IR.
type SubnetFact struct {
	NodeID           string `json:"node_id"`
	AvailabilityZone string `json:"availability_zone,omitempty"`
	// Region is derived from AvailabilityZone and only when that code has the documented
	// shape (see regionOfAZ); empty otherwise.
	Region     string           `json:"region,omitempty"`
	Visibility SubnetVisibility `json:"visibility"`
	Reason     string           `json:"reason,omitempty"`
}

// azCodeRE is the documented shape of an Availability Zone code: "Each Region has multiple,
// isolated locations known as Availability Zones. The code for an Availability Zone is its
// Region code followed by a letter identifier. For example, us-east-1a." — EC2 User Guide,
// "Regions and Zones" (docs.aws.amazon.com/AWSEC2/latest/UserGuide/
// using-regions-availability-zones.html). Local Zone ("us-west-2-lax-1") and Wavelength Zone
// ("us-east-1-wl1-bos-wlz-1") codes have different shapes and deliberately do NOT match: no
// region is derived for them rather than one guessed.
var azCodeRE = regexp.MustCompile(`^([a-z]{2,}(?:-[a-z]+)+-[0-9]+)[a-z]$`)

// RegionOfAZ returns the region code of an AZ code of the documented shape, or ok=false.
func RegionOfAZ(az string) (region string, ok bool) {
	m := azCodeRE.FindStringSubmatch(az)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// DeriveSubnetFacts returns one fact per structural subnet, sorted by node ID (NFR-1). A
// subnet is a network_boundary node that either declares an availability_zone (a VPC never
// does) or has a cidr_block and is contained in another node that also has one (its VPC).
// Public/private comes from PC-111's route evaluation (IsPublicSubnet): a subnet is public
// when its route table has a route to an internet gateway — AWS's own definition ("Public
// subnet – The subnet has a direct route to an internet gateway", VPC User Guide, "Subnets
// for your VPC").
func DeriveSubnetFacts(ir *IR) []SubnetFact {
	hasCIDR := map[string]bool{}
	for _, n := range ir.Nodes {
		if c, _ := n.RawAttributes["cidr_block"].(string); c != "" {
			hasCIDR[n.ID] = true
		}
	}
	parentHasCIDR := map[string]bool{}
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeContainedIn && hasCIDR[e.To] {
			parentHasCIDR[e.From] = true
		}
	}

	out := []SubnetFact{}
	for _, n := range ir.Nodes {
		if n.Type != NodeTypeNetworkBoundary {
			continue
		}
		az, _ := n.RawAttributes["availability_zone"].(string)
		if az == "" && !(hasCIDR[n.ID] && parentHasCIDR[n.ID]) {
			continue
		}
		f := SubnetFact{NodeID: n.ID, AvailabilityZone: az}
		if r, ok := RegionOfAZ(az); ok {
			f.Region = r
		}
		isPublic, hasRT := IsPublicSubnetIR(ir, n.ID)
		switch {
		case !hasRT:
			f.Visibility = SubnetVisibilityUnknown
			f.Reason = "no route table is associated with this subnet, so whether it routes to an internet gateway is unknown — never assumed private"
		case isPublic:
			f.Visibility = SubnetPublic
			f.Reason = "its route table has a route to an internet gateway"
		default:
			f.Visibility = SubnetPrivate
			f.Reason = "its route table has no route to an internet gateway"
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}
