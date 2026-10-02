// This file is PC-107's core-level half: the capability-level ladder itself, plus the
// honest per-NodeType ceiling this codebase's own real engines currently reach — I4
// applied to the catalog (PC-107's own Card): a service is never labelled as
// simulating behaviour no engine actually implements for its structural NodeType.
//
// Defined in core (not providers/) for the same reason NodeType/EdgeType are: the
// ladder itself is provider-agnostic vocabulary, and providers/mapping.go already
// imports core (never the reverse, per I1's "core is the pure engine everything else
// depends ON") — a provider's ResourceMapping declares core.CapabilityLevel the same
// way it already declares core.NodeType/core.EdgeType.
package core

// CapabilityLevel is PC-107's own named ladder, verbatim, from METADATA_ONLY (an
// unmodelled service — attributes captured, nothing else) up to FULL_BEHAVIOR (no
// service in this codebase claims this yet, honestly).
type CapabilityLevel string

const (
	CapabilityMetadataOnly      CapabilityLevel = "METADATA_ONLY"
	CapabilityConfiguration     CapabilityLevel = "CONFIGURATION"
	CapabilityValidation        CapabilityLevel = "VALIDATION"
	CapabilityConnectivity      CapabilityLevel = "CONNECTIVITY"
	CapabilityNetworkBehavior   CapabilityLevel = "NETWORK_BEHAVIOR"
	CapabilityIAMBehavior       CapabilityLevel = "IAM_BEHAVIOR"
	CapabilityRequestSimulation CapabilityLevel = "REQUEST_SIMULATION"
	CapabilityFailureSimulation CapabilityLevel = "FAILURE_SIMULATION"
	CapabilityFullBehavior      CapabilityLevel = "FULL_BEHAVIOR"
)

var capabilityRank = map[CapabilityLevel]int{
	CapabilityMetadataOnly:      0,
	CapabilityConfiguration:     1,
	CapabilityValidation:        2,
	CapabilityConnectivity:      3,
	CapabilityNetworkBehavior:   4,
	CapabilityIAMBehavior:       5,
	CapabilityRequestSimulation: 6,
	CapabilityFailureSimulation: 7,
	CapabilityFullBehavior:      8,
}

// Valid reports whether l is one of the 9 defined rungs.
func (l CapabilityLevel) Valid() bool {
	_, ok := capabilityRank[l]
	return ok
}

// AtLeast reports whether l reaches at least min on the ladder. An invalid level never
// reaches anything (I4: an unrecognized claim is not_assessable, never a pass).
func (l CapabilityLevel) AtLeast(min CapabilityLevel) bool {
	lr, ok := capabilityRank[l]
	if !ok {
		return false
	}
	mr, ok := capabilityRank[min]
	if !ok {
		return false
	}
	return lr >= mr
}

// MaxImplementedCapabilityLevel is the honest ceiling: the highest rung any REAL
// engine in this codebase currently reaches for a given structural NodeType,
// independent of which specific provider service maps to it. This is what
// providers.ResourceMapping.validate() checks a mapping's declared CapabilityLevel
// against at Load() time (PC-107's own acceptance criterion: "No service is labelled
// above its actual implemented level") — raising any of these ceilings without also
// shipping the engine that justifies it would be exactly the mislabeling I4 forbids.
//
// Every NodeType's floor is CONFIGURATION: ingest/build.go's buildNode captures a
// mapped resource's raw Terraform attributes onto RawAttributes unconditionally
// (subnet.yaml's own doc comment already establishes this), so "configuration
// captured" is real and true for anything with a mapping at all, never claimed above
// what a specific dedicated engine reaches:
//   - network_boundary: NETWORK_BEHAVIOR — real routing (PC-111), Security Group
//     (PC-112), and Network ACL (PC-113) evaluation engines.
//   - managed_database, cache: FAILURE_SIMULATION — a dedicated, cited Multi-AZ/
//     automatic-failover claim (providers/aws/rds.yaml, elasticache.yaml's own
//     FailoverMapping) beyond generic node-loss.
//   - container_workload, load_balancer, queue/stream, dns: REQUEST_SIMULATION — a
//     valid source/destination in PC-114's BuildTrace pipeline, but no dedicated
//     per-service failure claim of their own yet.
//   - compute: REQUEST_SIMULATION (PC-156) — a compute node placed in a subnet with
//     security groups is a valid source/destination in BuildTrace, and BuildOutboundTrace
//     decides its outbound path including public-address assignment. A mapping still
//     declares its OWN level (Lambda stays CONFIGURATION: no placement is resolved for it).
//   - object_store, identity, external_dependency: CONFIGURATION — no dedicated engine
//     touches these structurally yet (no S3/IAM-evaluation/external-dependency engine
//     exists); raising any of these requires shipping that engine first, not just adding
//     a palette entry.
func MaxImplementedCapabilityLevel(t NodeType) CapabilityLevel {
	switch t {
	case NodeTypeNetworkBoundary:
		return CapabilityNetworkBehavior
	case NodeTypeManagedDatabase, NodeTypeCache:
		return CapabilityFailureSimulation
	case NodeTypeContainerWorkload, NodeTypeCompute, NodeTypeLoadBalancer, NodeTypeQueueStream, NodeTypeDNS:
		return CapabilityRequestSimulation
	case NodeTypeObjectStore, NodeTypeIdentity, NodeTypeExternalDependency:
		return CapabilityConfiguration
	default:
		return CapabilityMetadataOnly
	}
}
