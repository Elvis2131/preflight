package cidr

// PC-106's real conformance tests — VPC/subnet CIDR sizing and reserved addresses.

import (
	"testing"

	"preflight/core"
	"preflight/tests/aws-conformance/harness"
)

func rules() core.CIDRRules {
	return core.CIDRRules{VPCMinPrefixLen: 16, VPCMaxPrefixLen: 28, SubnetMinPrefixLen: 16, SubnetMaxPrefixLen: 28, ReservedAddresses: 5}
}

func TestCIDR_VPCSizeRange_001(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "CIDR-VPC-RANGE-001",
		Rule:          "A VPC's IPv4 CIDR block must be between a /16 netmask (65,536 addresses) and a /28 netmask (16 addresses).",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html",
		Scenario:      "A VPC CIDR larger than /16 (more addresses than allowed).",
		Configuration: "vpc CIDR 10.0.0.0/8",
		Request:       "Allocate one subnet inside it.",
		Expected:      "Rejected as malformed — outside the allowed VPC size range.",
	})
	_, err := core.AllocateSubnets("10.0.0.0/8", []core.SubnetRequest{{Name: "a", MinUsableHosts: 10}}, rules())
	var cerr *core.CIDRError
	if err == nil {
		t.Fatalf("%s (%s): got no error, want rejection — see %s", spec.ID, spec.Rule, spec.Source)
	}
	if ok := castCIDRError(err, &cerr); !ok || cerr.Code != core.CIDRErrorMalformed {
		t.Fatalf("%s (%s): got %v, want CIDRErrorMalformed — see %s", spec.ID, spec.Rule, err, spec.Source)
	}
}

func TestCIDR_SubnetSizeRange_002(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "CIDR-SUBNET-RANGE-002",
		Rule:          "The allowed IPv4 CIDR block size for a subnet is between a /28 netmask and /16 netmask.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html",
		Scenario:      "An architect overrides a subnet to /29 — smaller than the allowed minimum.",
		Configuration: "vpc CIDR 10.0.0.0/16, override subnet 10.0.0.0/29",
		Request:       "Validate the override.",
		Expected:      "Rejected as undersized (smaller than /28).",
	})
	err := core.ValidateSubnetOverride("10.0.0.0/16", "10.0.0.0/29", nil, rules())
	if err == nil || err.Code != core.CIDRErrorUndersized {
		t.Fatalf("%s (%s): got %v, want CIDRErrorUndersized — see %s", spec.ID, spec.Rule, err, spec.Source)
	}
}

func TestCIDR_ReservedAddresses_003(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "CIDR-RESERVED-003",
		Rule:          "The first four IP addresses and the last IP address in each subnet CIDR block are not available for use — 5 reserved addresses per subnet.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html",
		Scenario:      "A /24 subnet (256 total addresses).",
		Configuration: "subnet 10.0.0.0/24",
		Request:       "Compute the usable address count.",
		Expected:      "251 usable addresses (256 - 5 reserved), per AWS's own worked example.",
	})
	result, err := core.AllocateSubnets("10.0.0.0/16", []core.SubnetRequest{{Name: "a", MinUsableHosts: 251}}, rules())
	if err != nil {
		t.Fatalf("%s (%s): AllocateSubnets: %v — see %s", spec.ID, spec.Rule, err, spec.Source)
	}
	if result[0].CIDR != "10.0.0.0/24" {
		t.Fatalf("%s (%s): got CIDR %s, want a /24 to fit exactly 251 usable hosts — see %s", spec.ID, spec.Rule, result[0].CIDR, spec.Source)
	}
	if result[0].UsableAddresses != 251 {
		t.Fatalf("%s (%s): got %d usable addresses, want 251 — see %s", spec.ID, spec.Rule, result[0].UsableAddresses, spec.Source)
	}
}

func TestCIDR_NoOverlapWithinVPC_004(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "CIDR-OVERLAP-004",
		Rule:          "If you create more than one subnet in a VPC, the CIDR blocks of the subnets cannot overlap.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html",
		Scenario:      "An architect overrides a new subnet to a range that overlaps an existing one.",
		Configuration: "vpc CIDR 10.0.0.0/16, existing subnet 10.0.0.0/24, override subnet 10.0.0.128/25",
		Request:       "Validate the override.",
		Expected:      "Rejected as overlapping.",
	})
	err := core.ValidateSubnetOverride("10.0.0.0/16", "10.0.0.128/25", []string{"10.0.0.0/24"}, rules())
	if err == nil || err.Code != core.CIDRErrorOverlapping {
		t.Fatalf("%s (%s): got %v, want CIDRErrorOverlapping — see %s", spec.ID, spec.Rule, err, spec.Source)
	}
}

func TestCIDR_SubnetMustFallInsideVPC_005(t *testing.T) {
	spec := harness.Verify(t, harness.Spec{
		ID:            "CIDR-INVPC-005",
		Rule:          "The CIDR block of a subnet is a subset of the CIDR block for the VPC.",
		Source:        "https://docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html",
		Scenario:      "An architect overrides a subnet to a range entirely outside the VPC's own CIDR.",
		Configuration: "vpc CIDR 10.0.0.0/16, override subnet 10.1.0.0/24",
		Request:       "Validate the override.",
		Expected:      "Rejected as out-of-VPC.",
	})
	err := core.ValidateSubnetOverride("10.0.0.0/16", "10.1.0.0/24", nil, rules())
	if err == nil || err.Code != core.CIDRErrorOutOfVPC {
		t.Fatalf("%s (%s): got %v, want CIDRErrorOutOfVPC — see %s", spec.ID, spec.Rule, err, spec.Source)
	}
}

func castCIDRError(err error, out **core.CIDRError) bool {
	cerr, ok := err.(*core.CIDRError)
	if !ok {
		return false
	}
	*out = cerr
	return true
}
