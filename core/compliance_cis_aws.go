// This file is PC-121's "broader CIS AWS" half — deliberately NOT expanded with new
// controls beyond what PC-18/28 already built. Investigated before writing any code:
// PC-18's own StorageEncryptionCheck doc comment already records a "NAMED GAP, carried
// forward unchanged from PC-18: a citable CIS AWS Foundations Benchmark control ID
// specifically covering RDS storage encryption was not found via any
// publicly-accessible source." Sourcing new, accurate CIS AWS Foundations Benchmark
// control IDs (the numbering itself is public, but verifying each one maps correctly
// to a real, current benchmark section without access to the actual benchmark
// document) carries exactly the same citation risk this project's "uncertain stays
// unknown, never guessed" discipline forbids taking on lightly — especially for a
// framework whose entire purpose is auditor-facing accuracy. Rather than invent CIS
// control IDs from general knowledge with no way to verify them here, this file
// exposes PC-18/28's own two ALREADY-VERIFIED checks (storage encryption technical
// fact, NAT gateway redundancy) through this ticket's new catalog structure — giving
// CIS AWS real, honest framework-selectable support without fabricating new
// citations. Expanding the CIS AWS catalog further with newly-sourced, verified
// control IDs is real, separate future work, not attempted here.
package core

import "strings"

// BuildCISAWSCatalog exposes PC-18/28's own two existing, already-verified checks
// through PC-121's catalog structure. Both are classified assessable_from_architecture
// — they always were; this ticket only gives them a citable requirement ID(ish) slot
// and a place in the counted totals PC-110's framework selector needs.
func BuildCISAWSCatalog(ir *IR, workload Workload) []ComplianceControlResult {
	prov := NewProvenance(KindDerived, "core/compliance_cis_aws")
	var results []ComplianceControlResult

	var databases []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeManagedDatabase {
			databases = append(databases, n)
		}
	}
	if len(databases) == 0 {
		results = append(results, notAssessableFromArchitectureResultWithRationale(
			"cis_aws.storage_encryption", FrameworkCISAWS, "unmapped",
			"Managed database storage encryption at rest", prov, "this IR has no managed_database node to evaluate"))
	}
	for _, db := range databases {
		var encrypted *bool
		if db.Capability != nil && db.Capability.EncryptionMechanism != nil {
			v := *db.Capability.EncryptionMechanism == "true"
			encrypted = &v
		}
		status, rationale := StorageEncryptionCheck(encrypted, prov)
		nodeID := db.ID
		results = append(results, ComplianceControlResult{
			ControlID: "cis_aws.storage_encryption", Framework: FrameworkCISAWS, RequirementID: "unmapped",
			// RequirementID "unmapped" is honest, not a placeholder to fill in later
			// without re-verifying: PC-18's own doc comment already recorded that no
			// citable CIS AWS Foundations Benchmark control ID was found for this
			// check via any publicly-accessible source.
			Title: "Managed database storage encryption at rest", Classification: ClassificationAssessable,
			NodeID: nodeID, Result: status, Rationale: rationale,
		})
	}

	results = append(results, natGatewayRedundancyComplianceResult(ir, prov))

	return results
}

func natGatewayRedundancyComplianceResult(ir *IR, prov Provenance) ComplianceControlResult {
	if unread := anyUnresolved(ir, AffectsPlacement); len(unread) > 0 {
		return ComplianceControlResult{
			ControlID: "cis_aws.nat_gateway_redundancy", Framework: FrameworkCISAWS, RequirementID: "unmapped",
			Title: "NAT gateway redundancy across public subnets", Classification: ClassificationAssessable,
			Result:    ComplianceResult{Status: ComplianceNotAssessable, Provenance: prov},
			Rationale: "a placement input could not be read, so which subnet each NAT gateway lives in is unknown: " + strings.Join(unread, "; "),
		}
	}
	var containmentEdges []DirectedEdge
	for _, e := range ir.Edges {
		if e.Type == EdgeTypeContainedIn {
			containmentEdges = append(containmentEdges, DirectedEdge{From: e.From, To: e.To})
		}
	}
	nodeTypeByID := map[string]NodeType{}
	for _, n := range ir.Nodes {
		nodeTypeByID[n.ID] = n.Type
	}
	counts := map[string]int{}
	for _, subnet := range goldenPublicSubnets {
		if _, present := nodeTypeByID[subnet]; !present {
			continue
		}
		count := 0
		for _, id := range ContainmentBlastRadius(containmentEdges, subnet) {
			if nodeTypeByID[id] == NodeTypeNetworkBoundary && isNATGatewayID(id) {
				count++
			}
		}
		counts[subnet] = count
	}
	result, rationale := NATGatewayRedundancyCheck(counts, prov)
	return ComplianceControlResult{
		ControlID: "cis_aws.nat_gateway_redundancy", Framework: FrameworkCISAWS, RequirementID: "unmapped",
		Title: "NAT gateway redundancy across public subnets", Classification: ClassificationAssessable,
		Result: result, Rationale: rationale,
	}
}
