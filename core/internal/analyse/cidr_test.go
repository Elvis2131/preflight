package analyse

import (
	"math/rand"
	"net/netip"
	"testing"
)

func awsRules() CIDRRules {
	return CIDRRules{VPCMinPrefixLen: 16, VPCMaxPrefixLen: 28, SubnetMinPrefixLen: 16, SubnetMaxPrefixLen: 28, ReservedAddresses: 5}
}

func TestUsableHosts_ReflectsAWSReservedAddresses(t *testing.T) {
	// A /24 has 256 total addresses; AWS reserves 5, so 251 usable — the documented
	// worked example's own arithmetic (docs.aws.amazon.com/vpc/latest/userguide/
	// subnet-sizing.html).
	if got := usableHosts(24, 5); got != 251 {
		t.Errorf("usableHosts(24, 5) = %d, want 251", got)
	}
}

func TestAllocateSubnets_NonOverlappingAndInVPC(t *testing.T) {
	requests := []SubnetRequest{
		{Name: "public-a", MinUsableHosts: 200},
		{Name: "public-b", MinUsableHosts: 200},
		{Name: "private-a", MinUsableHosts: 50},
		{Name: "private-b", MinUsableHosts: 50},
	}
	result, err := AllocateSubnets("10.0.0.0/16", requests, awsRules())
	if err != nil {
		t.Fatalf("AllocateSubnets: %v", err)
	}
	if len(result) != len(requests) {
		t.Fatalf("got %d results, want %d", len(result), len(requests))
	}

	vpc := netip.MustParsePrefix("10.0.0.0/16")
	var prefixes []netip.Prefix
	for i, r := range result {
		if r.Name != requests[i].Name {
			t.Errorf("result[%d].Name = %q, want %q (must preserve input order)", i, r.Name, requests[i].Name)
		}
		p, err := netip.ParsePrefix(r.CIDR)
		if err != nil {
			t.Fatalf("result CIDR %q doesn't parse: %v", r.CIDR, err)
		}
		if !vpc.Contains(p.Addr()) {
			t.Errorf("subnet %s (%s) is not inside VPC %s", r.Name, r.CIDR, vpc)
		}
		for _, other := range prefixes {
			if p.Overlaps(other) {
				t.Errorf("subnet %s (%s) overlaps an earlier allocation (%s)", r.Name, r.CIDR, other)
			}
		}
		prefixes = append(prefixes, p)
	}
}

func TestAllocateSubnets_Deterministic(t *testing.T) {
	requests := []SubnetRequest{
		{Name: "a", MinUsableHosts: 500},
		{Name: "b", MinUsableHosts: 10},
		{Name: "c", MinUsableHosts: 1000},
	}
	r1, err := AllocateSubnets("10.0.0.0/16", requests, awsRules())
	if err != nil {
		t.Fatalf("AllocateSubnets: %v", err)
	}
	r2, err := AllocateSubnets("10.0.0.0/16", requests, awsRules())
	if err != nil {
		t.Fatalf("AllocateSubnets: %v", err)
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Fatalf("same input twice produced different results at index %d: %+v vs %+v", i, r1[i], r2[i])
		}
	}
}

func TestAllocateSubnets_PropertyTest_NoOverlapNoOutOfRange(t *testing.T) {
	rng := rand.New(rand.NewSource(42)) // fixed seed: deterministic across runs
	for trial := 0; trial < 200; trial++ {
		n := 1 + rng.Intn(20)
		requests := make([]SubnetRequest, n)
		for i := range requests {
			requests[i] = SubnetRequest{Name: string(rune('a'+(i%26))) + string(rune('0'+trial%10)) + string(rune('A'+i/26)), MinUsableHosts: 1 + rng.Intn(2000)}
		}
		result, err := AllocateSubnets("10.0.0.0/16", requests, awsRules())
		if err != nil {
			// A /16 VPC can't always fit 20 arbitrarily large subnets — a real
			// capacity error is a correct outcome, not a property violation.
			continue
		}
		vpc := netip.MustParsePrefix("10.0.0.0/16")
		var prefixes []netip.Prefix
		for _, r := range result {
			p, err := netip.ParsePrefix(r.CIDR)
			if err != nil {
				t.Fatalf("trial %d: result CIDR %q doesn't parse: %v", trial, r.CIDR, err)
			}
			if !vpc.Contains(p.Addr()) || !vpc.Contains(lastAddr(p)) {
				t.Fatalf("trial %d: subnet %s is out of VPC range", trial, r.CIDR)
			}
			for _, other := range prefixes {
				if p.Overlaps(other) {
					t.Fatalf("trial %d: subnet %s overlaps %s", trial, r.CIDR, other)
				}
			}
			prefixes = append(prefixes, p)
		}
	}
}

func TestValidateSubnetOverride_Undersized(t *testing.T) {
	err := ValidateSubnetOverride("10.0.0.0/16", "10.0.0.0/29", nil, awsRules())
	if err == nil || err.Code != CIDRErrorUndersized {
		t.Fatalf("got %v, want CIDRErrorUndersized", err)
	}
}

func TestValidateSubnetOverride_Oversized(t *testing.T) {
	err := ValidateSubnetOverride("10.0.0.0/8", "10.0.0.0/8", nil, awsRules())
	if err == nil || err.Code != CIDRErrorOversized {
		t.Fatalf("got %v, want CIDRErrorOversized", err)
	}
}

func TestValidateSubnetOverride_OutOfVPC(t *testing.T) {
	err := ValidateSubnetOverride("10.0.0.0/16", "10.1.0.0/24", nil, awsRules())
	if err == nil || err.Code != CIDRErrorOutOfVPC {
		t.Fatalf("got %v, want CIDRErrorOutOfVPC", err)
	}
}

func TestValidateSubnetOverride_Overlapping(t *testing.T) {
	err := ValidateSubnetOverride("10.0.0.0/16", "10.0.0.128/25", []string{"10.0.0.0/24"}, awsRules())
	if err == nil || err.Code != CIDRErrorOverlapping {
		t.Fatalf("got %v, want CIDRErrorOverlapping", err)
	}
}

func TestValidateSubnetOverride_Valid(t *testing.T) {
	err := ValidateSubnetOverride("10.0.0.0/16", "10.0.1.0/24", []string{"10.0.0.0/24"}, awsRules())
	if err != nil {
		t.Fatalf("got %v, want no error (valid, non-overlapping, in-VPC override)", err)
	}
}
