// This file is PC-121's PCI DSS catalog, built against PCI DSS v4.0.1 (read locally; the standard
// is licensed and is NOT stored here: only requirement IDs, short titles written in our own words,
// and a classification).
//
// SCOPE, stated rather than implied. The standard has 12 principal requirements and roughly 250
// numbered sub-requirements. Each architecture-relevant sub-requirement is its own row, cited by its
// real v4.0.1 number, and classified:
//   - 1.4.4  assessable: systems storing cardholder data are not directly reachable from untrusted networks
//   - 1.3.1, 1.3.2, 3.5.1.2, 4.2.1, 6.4.2, 7.2.2  partially assessable: the architecture supports
//     them, but the full requirement also needs documentation or process evidence
//
// Every principal requirement then also has one "remaining sub-requirements" row,
// not_assessable_from_architecture, so the denominator stays honest: nobody can read the architectural
// slice as the whole of a requirement. Requirements 2, 5, 8, 9, 10, 11 and 12 have no architecture-
// evidenced sub-requirement in this model and are single not-assessable rows.
//
// A not_assessable row carries no evaluation logic at all, so it can never report satisfied.
//
// CORRECTION recorded: the earlier catalog reported "Requirement 3: storage encryption: satisfied"
// for encrypted database storage. v4.0.1 3.5.1.2 says disk-level or partition-level encryption renders
// account data unreadable only on removable media, or on non-removable media when the data is also
// rendered unreadable another way. Storage encryption is therefore supporting evidence (applicable),
// never a pass. See checkStoredDataProtection.
package core

const pciFramework = FrameworkPCIDSS4

// BuildPCIDSS4Catalog runs every PCI DSS control this catalog declares against ir.
func BuildPCIDSS4Catalog(ir *IR, workload Workload) []ComplianceControlResult {
	prov := NewProvenance(KindDerived, "core/compliance_pci_dss")
	rest := func(req, what string) ComplianceControlResult {
		return notAssessableFromArchitectureResult("pci_dss_4:"+req+".rest", pciFramework, req, "Rest of Requirement "+req+": "+what, prov)
	}
	whole := func(req, title string) ComplianceControlResult {
		return notAssessableFromArchitectureResult("pci_dss_4:"+req, pciFramework, req, title, prov)
	}
	c := func(req, title string, class ControlClassification) ctl {
		return ctl{id: "pci_dss_4:" + req, framework: pciFramework, req: req, title: title, class: class}
	}

	var out []ComplianceControlResult
	// Requirement 1: network security controls
	out = append(out, checkDatabaseSGIngress(ir, c("1.3.1", "Inbound traffic to the cardholder environment is limited to what is necessary", ClassificationPartial), prov)...)
	out = append(out, checkDatabaseSGEgress(ir, c("1.3.2", "Outbound traffic from the cardholder environment is limited to what is necessary", ClassificationPartial), prov)...)
	out = append(out, checkDatabaseNotPubliclyRoutable(ir, c("1.4.4", "Systems storing cardholder data are not directly reachable from untrusted networks", ClassificationAssessable), prov)...)
	out = append(out, rest("1", "documentation, rule approval and review, other network controls"))
	// Requirement 2
	out = append(out, whole("2", "Secure configurations applied to all system components"))
	// Requirement 3: protect stored account data
	out = append(out, checkStoredDataProtection(ir, c("3.5.1.2", "Stored account data is unreadable; disk-level encryption alone is not enough on non-removable media", ClassificationPartial), prov,
		"PCI DSS 3.5.1.2 accepts disk-level encryption on non-removable media only when the data is also rendered unreadable another way")...)
	out = append(out, rest("3", "retention, masking, key management, other stored-data controls"))
	// Requirement 4: cryptography in transit
	out = append(out, checkTransportNotModelled(c("4.2.1", "Strong cryptography protects account data sent over open public networks", ClassificationPartial), prov)...)
	out = append(out, rest("4", "certificate inventory, other transmission controls"))
	// Requirement 5
	out = append(out, whole("5", "Systems and networks protected from malicious software"))
	// Requirement 6
	out = append(out, checkPublicFacingBehindWAF(ir, c("6.4.2", "Public-facing web applications sit behind an automated attack detection and prevention layer", ClassificationPartial), prov)...)
	out = append(out, rest("6", "secure development, vulnerability handling, change control"))
	// Requirement 7
	out = append(out, checkLeastPrivilege(ir, c("7.2.2", "Access is granted by job function using least privilege", ClassificationPartial), prov)...)
	out = append(out, rest("7", "access model, account reviews, other access controls"))
	// Requirements with no architecture-evidenced sub-requirement in this model
	out = append(out, whole("8", "Users identified and authenticated before access"))
	out = append(out, whole("9", "Physical access to cardholder data restricted"))
	out = append(out, whole("10", "Access to system components and cardholder data logged and monitored"))
	out = append(out, whole("11", "Security of systems and networks tested regularly"))
	out = append(out, whole("12", "Policies and programs support information security"))
	return out
}
