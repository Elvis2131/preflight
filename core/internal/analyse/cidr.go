// This file is PC-106: a pure (no I/O — ADR-004/NFR-1) CIDR allocator and validator,
// ported from the reference project's engine/layout/cidrAllocator.ts as a first draft
// (ADR-004: port, don't vendor), then re-verified against AWS's own VPC documentation
// rather than trusted as ground truth (the reference repo has shipped correctness
// bugs, per this ticket's own Conversation) — every numeric rule below is a parameter
// (CIDRRules), sourced by the caller from providers/aws/cidr_rules.go's own real,
// cited constants, never hardcoded here, so this engine stays provider-agnostic.
package analyse

import (
	"fmt"
	"net/netip"
	"sort"
)

// CIDRRules is the provider-supplied sizing/reservation policy this engine validates
// against — see providers/aws/cidr_rules.go for AWS's own real, cited values.
type CIDRRules struct {
	VPCMinPrefixLen    int
	VPCMaxPrefixLen    int
	SubnetMinPrefixLen int
	SubnetMaxPrefixLen int
	ReservedAddresses  int
}

// SubnetRequest is one subnet an architect wants allocated. MinUsableHosts is the
// number of usable addresses it must accommodate; the allocator picks the smallest
// AWS-legal subnet size that satisfies it after reserved addresses are subtracted.
type SubnetRequest struct {
	Name           string
	MinUsableHosts int
}

// AllocatedSubnet is one request's real result.
type AllocatedSubnet struct {
	Name            string
	CIDR            string
	UsableAddresses int
}

// CIDRErrorCode names a distinct, testable rejection reason — PC-106's own acceptance
// criterion: "Undersized, oversized, overlapping, and out-of-VPC overrides each
// rejected with a distinct error code."
type CIDRErrorCode string

const (
	CIDRErrorUndersized   CIDRErrorCode = "cidr_undersized"
	CIDRErrorOversized    CIDRErrorCode = "cidr_oversized"
	CIDRErrorOverlapping  CIDRErrorCode = "cidr_overlapping"
	CIDRErrorOutOfVPC     CIDRErrorCode = "cidr_out_of_vpc"
	CIDRErrorMalformed    CIDRErrorCode = "cidr_malformed"
	CIDRErrorInsufficient CIDRErrorCode = "cidr_insufficient_vpc_space"
)

// CIDRError is a structured rejection naming which rule was violated and why —
// distinguishable by Code, human-readable by Error().
type CIDRError struct {
	Code    CIDRErrorCode
	Message string
}

func (e *CIDRError) Error() string { return e.Message }

// usableHosts returns how many addresses a subnet of this prefix length has left
// after AWS's own reserved addresses are subtracted (providers/aws/cidr_rules.go's
// own cited SubnetReservedAddresses).
func usableHosts(prefixLen int, reserved int) int {
	total := 1 << uint(32-prefixLen)
	return total - reserved
}

// prefixLenForHosts returns the largest (most restrictive) subnet prefix length whose
// usable address count still covers minHosts, clamped to the rules' allowed range.
func prefixLenForHosts(minHosts int, rules CIDRRules) (int, error) {
	for p := rules.SubnetMaxPrefixLen; p >= rules.SubnetMinPrefixLen; p-- {
		if usableHosts(p, rules.ReservedAddresses) >= minHosts {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no AWS-legal subnet size (/%d-/%d) accommodates %d usable hosts", rules.SubnetMinPrefixLen, rules.SubnetMaxPrefixLen, minHosts)
}

// AllocateSubnets deterministically assigns non-overlapping, in-VPC ranges to every
// request — PC-106's own acceptance criterion: "the same set of subnets in the same
// order always gets the same ranges." Requests are processed largest-first (a stable
// sort by size descending, then Name ascending to break ties — never map iteration,
// per the Card's own instruction) for efficient packing, but results are returned in
// the caller's original input order.
func AllocateSubnets(vpcCIDR string, requests []SubnetRequest, rules CIDRRules) ([]AllocatedSubnet, error) {
	vpc, err := netip.ParsePrefix(vpcCIDR)
	if err != nil {
		return nil, &CIDRError{Code: CIDRErrorMalformed, Message: fmt.Sprintf("vpc CIDR %q is not a valid CIDR: %v", vpcCIDR, err)}
	}
	vpc = vpc.Masked()
	if vpc.Bits() < rules.VPCMinPrefixLen || vpc.Bits() > rules.VPCMaxPrefixLen {
		return nil, &CIDRError{Code: CIDRErrorMalformed, Message: fmt.Sprintf("vpc CIDR %s: allowed size is /%d to /%d", vpcCIDR, rules.VPCMinPrefixLen, rules.VPCMaxPrefixLen)}
	}

	type sized struct {
		SubnetRequest
		prefixLen int
	}
	toPlace := make([]sized, 0, len(requests))
	for _, r := range requests {
		p, err := prefixLenForHosts(r.MinUsableHosts, rules)
		if err != nil {
			return nil, &CIDRError{Code: CIDRErrorInsufficient, Message: fmt.Sprintf("subnet %q: %v", r.Name, err)}
		}
		toPlace = append(toPlace, sized{r, p})
	}

	processOrder := make([]sized, len(toPlace))
	copy(processOrder, toPlace)
	sort.SliceStable(processOrder, func(i, j int) bool {
		if processOrder[i].prefixLen != processOrder[j].prefixLen {
			return processOrder[i].prefixLen < processOrder[j].prefixLen // smaller prefix len = bigger subnet = placed first
		}
		return processOrder[i].Name < processOrder[j].Name
	})

	var placed []netip.Prefix
	resultByName := map[string]AllocatedSubnet{}
	for _, req := range processOrder {
		cidr, ok := firstFit(vpc, req.prefixLen, placed)
		if !ok {
			return nil, &CIDRError{Code: CIDRErrorInsufficient, Message: fmt.Sprintf("no room left in %s for subnet %q (/%d)", vpcCIDR, req.Name, req.prefixLen)}
		}
		placed = append(placed, cidr)
		resultByName[req.Name] = AllocatedSubnet{Name: req.Name, CIDR: cidr.String(), UsableAddresses: usableHosts(req.prefixLen, rules.ReservedAddresses)}
	}

	out := make([]AllocatedSubnet, len(requests))
	for i, r := range requests {
		out[i] = resultByName[r.Name]
	}
	return out, nil
}

// firstFit scans the VPC's address space in ascending, aligned steps of the requested
// subnet size and returns the first block that doesn't overlap any already-placed one
// — a deterministic, order-independent-of-map-iteration search (a plain slice scan).
func firstFit(vpc netip.Prefix, prefixLen int, placed []netip.Prefix) (netip.Prefix, bool) {
	step := uint64(1) << uint(32-prefixLen)
	base := prefixToUint32(vpc)
	vpcSize := uint64(1) << uint(32-vpc.Bits())

	for offset := uint64(0); offset < vpcSize; offset += step {
		candidateAddr := base + offset
		candidate := netip.PrefixFrom(uint32ToAddr(uint32(candidateAddr)), prefixLen)
		overlapsAny := false
		for _, p := range placed {
			if candidate.Overlaps(p) {
				overlapsAny = true
				break
			}
		}
		if !overlapsAny {
			return candidate, true
		}
	}
	return netip.Prefix{}, false
}

func prefixToUint32(p netip.Prefix) uint64 {
	a4 := p.Masked().Addr().As4()
	return uint64(a4[0])<<24 | uint64(a4[1])<<16 | uint64(a4[2])<<8 | uint64(a4[3])
}

func uint32ToAddr(v uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// ValidateSubnetOverride checks an architect-supplied (not auto-allocated) subnet CIDR
// against every AWS rule this engine knows, returning a *CIDRError naming the FIRST
// violated rule — PC-106's own acceptance criterion: each rejection reason is a
// distinct, testable code.
func ValidateSubnetOverride(vpcCIDR, subnetCIDR string, existing []string, rules CIDRRules) *CIDRError {
	vpc, err := netip.ParsePrefix(vpcCIDR)
	if err != nil {
		return &CIDRError{Code: CIDRErrorMalformed, Message: fmt.Sprintf("vpc CIDR %q is not a valid CIDR: %v", vpcCIDR, err)}
	}
	subnet, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return &CIDRError{Code: CIDRErrorMalformed, Message: fmt.Sprintf("subnet CIDR %q is not a valid CIDR: %v", subnetCIDR, err)}
	}
	vpc, subnet = vpc.Masked(), subnet.Masked()

	if subnet.Bits() < rules.SubnetMinPrefixLen {
		return &CIDRError{Code: CIDRErrorOversized, Message: fmt.Sprintf("subnet %s is larger than the maximum allowed /%d", subnetCIDR, rules.SubnetMinPrefixLen)}
	}
	if subnet.Bits() > rules.SubnetMaxPrefixLen {
		return &CIDRError{Code: CIDRErrorUndersized, Message: fmt.Sprintf("subnet %s is smaller than the minimum allowed /%d", subnetCIDR, rules.SubnetMaxPrefixLen)}
	}
	if subnet.Bits() < vpc.Bits() || !vpc.Contains(subnet.Addr()) || !vpc.Contains(lastAddr(subnet)) {
		return &CIDRError{Code: CIDRErrorOutOfVPC, Message: fmt.Sprintf("subnet %s does not fall entirely inside VPC CIDR %s", subnetCIDR, vpcCIDR)}
	}
	for _, e := range existing {
		ep, err := netip.ParsePrefix(e)
		if err != nil {
			continue
		}
		if subnet.Overlaps(ep.Masked()) {
			return &CIDRError{Code: CIDRErrorOverlapping, Message: fmt.Sprintf("subnet %s overlaps existing subnet %s", subnetCIDR, e)}
		}
	}
	return nil
}

func lastAddr(p netip.Prefix) netip.Addr {
	base := prefixToUint32(p)
	size := uint64(1) << uint(32-p.Bits())
	return uint32ToAddr(uint32(base + size - 1))
}
