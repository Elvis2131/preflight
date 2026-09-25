// This file is PC-106's provider-data half: AWS's own documented VPC/subnet CIDR
// sizing and reservation rules, kept here (not hardcoded in core) so core's CIDR
// allocator (core/internal/analyse/cidr.go) stays provider-agnostic — the same
// "rule constants live in providers/<name>, logic lives in core" split this codebase
// already uses for capability/failover mappings (PC-13, PC-22). Every value below is a
// plain numeric fact, not a resource-to-IR mapping, so it does not fit the
// ResourceMapping YAML schema mapping.go's Load() validates — a small Go const block
// is the honest shape for provider-wide numeric facts, not a second YAML-loading
// mechanism built for a single ticket's needs.
package aws

// VPCMinPrefixLen/VPCMaxPrefixLen: "the allowed block size is between a /16 netmask
// (65,536 IP addresses) and /28 netmask (16 IP addresses)."
// (docs.aws.amazon.com/vpc/latest/userguide/vpc-cidr-blocks.html, "IPv4 VPC CIDR
// blocks", verified 2026-09-25)
const (
	VPCMinPrefixLen = 16
	VPCMaxPrefixLen = 28
)

// SubnetMinPrefixLen/SubnetMaxPrefixLen: "The allowed IPv4 CIDR block size for a
// subnet is between a /28 netmask and /16 netmask."
// (docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html, "Subnet sizing for
// IPv4", verified 2026-09-25)
const (
	SubnetMinPrefixLen = 16
	SubnetMaxPrefixLen = 28
)

// SubnetReservedAddresses: "The first four IP addresses and the last IP address in
// each subnet CIDR block are not available for your use" — network address, the VPC
// router, the DNS server (base+2), reserved for future use, and the network broadcast
// address (not supported in a VPC, but still reserved).
// (docs.aws.amazon.com/vpc/latest/userguide/subnet-sizing.html, "Subnet sizing for
// IPv4", verified 2026-09-25)
const SubnetReservedAddresses = 5
