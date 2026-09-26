// This file is PC-121's PCI DSS v4.0 catalog. SCOPE, stated explicitly rather than
// silently partial: this catalogs the standard's own 12 TOP-LEVEL requirement
// numbers only, not its ~300 individual numbered sub-requirements (e.g. 3.5.1,
// 1.2.8). Going to sub-requirement granularity would require verifying dozens of
// individual citations against the full paid standard text this project has no
// access to republish or verify precisely — the same "uncertain stays unknown, never
// guessed" discipline PC-18's own storage-encryption check already applied to a CIS
// control ID it could not source. Two of the twelve are further classified
// assessable_from_architecture, reusing existing engine logic verbatim (StorageEncryptionCheck,
// IsPublicSubnet) — zero new evaluation logic invented for this ticket. The rest are
// not_assessable_from_architecture: PCI DSS is majority process/operational/physical
// controls (secure coding practices, incident response, vendor management, badge
// access), which no Terraform diagram can ever evidence, exactly as the Card's own
// Conversation anticipates.
package core

// BuildPCIDSS4Catalog runs every PCI DSS v4.0 control this catalog declares against
// ir/workload. Requirement titles are short, independently-paraphrased summaries of
// each requirement's own public numbering (see compliance_catalog.go's own doc
// comment for the citation) — never the standard's verbatim text.
func BuildPCIDSS4Catalog(ir *IR, workload Workload) []ComplianceControlResult {
	prov := NewProvenance(KindDerived, "core/compliance_pci_dss")
	var results []ComplianceControlResult

	results = append(results, pciReq1NetworkSecurity(ir, prov)...)
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_2", FrameworkPCIDSS4, "2", "Apply secure configurations to all system components", prov))
	results = append(results, pciReq3ProtectStoredData(ir, prov)...)
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_4", FrameworkPCIDSS4, "4", "Protect cardholder data with strong cryptography during transmission", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_5", FrameworkPCIDSS4, "5", "Protect all systems and networks from malicious software", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_6", FrameworkPCIDSS4, "6", "Develop and maintain secure systems and software", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_7", FrameworkPCIDSS4, "7", "Restrict access to system components and cardholder data by business need to know", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_8", FrameworkPCIDSS4, "8", "Identify users and authenticate access to system components", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_9", FrameworkPCIDSS4, "9", "Restrict physical access to cardholder data", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_10", FrameworkPCIDSS4, "10", "Log and monitor all access to system components and cardholder data", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_11", FrameworkPCIDSS4, "11", "Test security of systems and networks regularly", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"pci_dss_4.req_12", FrameworkPCIDSS4, "12", "Support information security with organizational policies and programs", prov))

	return results
}

// pciReq1NetworkSecurity is Requirement 1's own architecturally-assessable slice:
// "Install and maintain network security controls" includes, per its own public
// scope, restricting inbound/outbound traffic to the cardholder data environment.
// Reuses core.IsPublicSubnet (core/routing.go) verbatim — a managed_database node
// sitting in a subnet with a real route to an Internet Gateway is a real, structural
// network-security-control gap; the requirement's OTHER aspects (firewall rule review
// cadence, documented data-flow diagrams, personal firewalls on remote-access
// devices) are process controls this classification does not claim to cover, hence
// this is classified assessable_from_architecture for the one sub-aspect actually
// checked, not the whole of Requirement 1.
func pciReq1NetworkSecurity(ir *IR, prov Provenance) []ComplianceControlResult {
	var out []ComplianceControlResult
	var databases []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeManagedDatabase {
			databases = append(databases, n)
		}
	}
	if len(databases) == 0 {
		return []ComplianceControlResult{notAssessableFromArchitectureResultWithRationale(
			"pci_dss_4.req_1.no_public_database_route", FrameworkPCIDSS4, "1",
			"Network security controls: database not directly reachable from the internet",
			prov, "this IR has no managed_database node to evaluate")}
	}
	for _, db := range databases {
		out = append(out, databaseNotPubliclyRoutableResult(ir, db, "pci_dss_4.req_1.no_public_database_route", FrameworkPCIDSS4, "1",
			"Network security controls: database not directly reachable from the internet", prov))
	}
	return out
}

// pciReq3ProtectStoredData is Requirement 3's own architecturally-assessable slice:
// "Protect stored account data" — reuses StorageEncryptionCheck (PC-18/29) verbatim,
// the same encrypted-at-rest fact already computed for the scorecard's own compliance
// dimension. Requirement 3's other aspects (data retention/disposal policy, PAN
// masking on display, key-management procedures) are process controls not covered.
func pciReq3ProtectStoredData(ir *IR, prov Provenance) []ComplianceControlResult {
	var out []ComplianceControlResult
	var databases []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeManagedDatabase {
			databases = append(databases, n)
		}
	}
	if len(databases) == 0 {
		return []ComplianceControlResult{notAssessableFromArchitectureResultWithRationale(
			"pci_dss_4.req_3.storage_encryption", FrameworkPCIDSS4, "3",
			"Protect stored account data: storage encryption at rest", prov, "this IR has no managed_database node to evaluate")}
	}
	for _, db := range databases {
		var encrypted *bool
		if db.Capability != nil && db.Capability.EncryptionMechanism != nil {
			v := *db.Capability.EncryptionMechanism == "true"
			encrypted = &v
		}
		status, rationale := StorageEncryptionCheck(encrypted, prov)
		nodeID := db.ID
		out = append(out, ComplianceControlResult{
			ControlID: "pci_dss_4.req_3.storage_encryption", Framework: FrameworkPCIDSS4, RequirementID: "3",
			Title: "Protect stored account data: storage encryption at rest", Classification: ClassificationAssessable,
			NodeID: nodeID, Result: status, Rationale: rationale,
		})
	}
	return out
}
