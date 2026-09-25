// This file is PC-106's core-level public mirror of core/internal/analyse's real CIDR
// allocator/validator (I1: core/internal/* is compiler-enforced private) — same split
// as core/securitygroups.go/core/nacl.go before it.
package core

import "preflight/core/internal/analyse"

type CIDRRules = analyse.CIDRRules
type SubnetRequest = analyse.SubnetRequest
type AllocatedSubnet = analyse.AllocatedSubnet
type CIDRError = analyse.CIDRError
type CIDRErrorCode = analyse.CIDRErrorCode

const (
	CIDRErrorUndersized   = analyse.CIDRErrorUndersized
	CIDRErrorOversized    = analyse.CIDRErrorOversized
	CIDRErrorOverlapping  = analyse.CIDRErrorOverlapping
	CIDRErrorOutOfVPC     = analyse.CIDRErrorOutOfVPC
	CIDRErrorMalformed    = analyse.CIDRErrorMalformed
	CIDRErrorInsufficient = analyse.CIDRErrorInsufficient
)

// AllocateSubnets is the public entry point to PC-106's real, deterministic CIDR
// allocator — see core/internal/analyse/cidr.go's own doc comment for the full
// AWS citations and design reasoning.
func AllocateSubnets(vpcCIDR string, requests []SubnetRequest, rules CIDRRules) ([]AllocatedSubnet, error) {
	return analyse.AllocateSubnets(vpcCIDR, requests, rules)
}

// ValidateSubnetOverride is the public entry point to PC-106's override validator.
func ValidateSubnetOverride(vpcCIDR, subnetCIDR string, existing []string, rules CIDRRules) *CIDRError {
	return analyse.ValidateSubnetOverride(vpcCIDR, subnetCIDR, existing, rules)
}
