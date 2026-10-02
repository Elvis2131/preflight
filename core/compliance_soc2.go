// This file is PC-121's SOC 2 catalog, built against the AICPA 2017 Trust Services Criteria (with
// revised points of focus, 2022), read locally; the criteria text is licensed and is NOT stored
// here: only criterion IDs, short titles written in our own words, and a classification.
//
// SCOPE, stated rather than implied. Security (the nine Common Criteria categories, CC1.1 to CC9.2,
// 33 criteria) is in every SOC 2 report; this catalog also covers Availability (A1.1 to A1.3) and
// Confidentiality (C1.1, C1.2). Processing Integrity and Privacy are optional categories and are not
// cataloged. Each criterion is its own row, cited by its real ID. Five are partially assessable
// (CC6.1, CC6.3, CC6.6, CC6.7, A1.2): the architecture supports them but every one also needs
// operating evidence. The rest are not_assessable_from_architecture (governance, people, process,
// monitoring and vendor practices), with no evaluation logic, so they can never report satisfied.
// No criterion is classified assessable: SOC 2 attests to controls operating over time, which a
// design cannot show.
package core

const soc2Framework = FrameworkSOC2

// BuildSOC2Catalog runs every SOC 2 control this catalog declares against ir.
func BuildSOC2Catalog(ir *IR, workload Workload) []ComplianceControlResult {
	prov := NewProvenance(KindDerived, "core/compliance_soc2")
	na := func(id, title string) ComplianceControlResult {
		return notAssessableFromArchitectureResult("soc2:"+id, soc2Framework, id, title, prov)
	}
	c := func(id, title string) ctl {
		return ctl{id: "soc2:" + id, framework: soc2Framework, req: id, title: title, class: ClassificationPartial}
	}

	var out []ComplianceControlResult
	out = append(out,
		na("CC1.1", "Commitment to integrity and ethical values"),
		na("CC1.2", "Board oversight independent of management"),
		na("CC1.3", "Structures, reporting lines and authorities established"),
		na("CC1.4", "Commitment to attract, develop and retain competent people"),
		na("CC1.5", "People held accountable for internal control duties"),
		na("CC2.1", "Relevant, quality information used for internal control"),
		na("CC2.2", "Control information communicated internally"),
		na("CC2.3", "Control matters communicated with external parties"),
		na("CC3.1", "Objectives specified clearly enough to assess risk"),
		na("CC3.2", "Risks identified and analyzed"),
		na("CC3.3", "Fraud potential considered in risk assessment"),
		na("CC3.4", "Significant changes affecting controls assessed"),
		na("CC4.1", "Ongoing and separate evaluations of controls"),
		na("CC4.2", "Control deficiencies communicated for correction"),
		na("CC5.1", "Control activities selected to mitigate risk"),
		na("CC5.2", "General technology controls selected and developed"),
		na("CC5.3", "Controls deployed through policies and procedures"),
	)
	out = append(out, checkStoredDataProtection(ir, c("CC6.1", "Logical access security architecture protects information assets, including encryption"), prov,
		"SOC 2 CC6.1 leaves the use of encryption to the entity's own risk strategy")...)
	out = append(out, na("CC6.2", "Users registered and authorized before credentials are issued"))
	out = append(out, checkLeastPrivilege(ir, c("CC6.3", "Access is role-based and follows least privilege"), prov)...)
	out = append(out,
		na("CC6.4", "Physical access to facilities and assets restricted"),
		na("CC6.5", "Assets and data securely disposed of"),
	)
	out = append(out, checkDatabaseNotPubliclyRoutable(ir, c("CC6.6", "Logical access measures protect against threats from outside the system boundary"), prov)...)
	out = append(out, checkTransportNotModelled(c("CC6.7", "Data transmission, movement and removal restricted and protected"), prov)...)
	out = append(out,
		na("CC6.8", "Unauthorized or malicious software prevented or detected"),
		na("CC7.1", "Configuration changes and new vulnerabilities detected"),
		na("CC7.2", "System components monitored for anomalies"),
		na("CC7.3", "Security events evaluated"),
		na("CC7.4", "Security incidents responded to"),
		na("CC7.5", "Recovery from security incidents"),
		na("CC8.1", "Changes authorized, tested, approved and implemented"),
		na("CC9.1", "Risk mitigation for business disruption"),
		na("CC9.2", "Vendor and business-partner risk managed"),
		na("A1.1", "Processing capacity monitored and managed"),
	)
	out = append(out, checkRecoveryInfrastructure(ir, c("A1.2", "Environmental protections, backups and recovery infrastructure in place"), prov)...)
	out = append(out,
		na("A1.3", "Recovery plan procedures tested"),
		na("C1.1", "Confidential information identified and maintained"),
		na("C1.2", "Confidential information disposed of"),
	)
	return out
}
