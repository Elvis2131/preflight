// This file is PC-121's SOC 2 catalog. SCOPE, stated explicitly: this catalogs the
// nine TOP-LEVEL Common Criteria categories (CC1-CC9) from the AICPA Trust Services
// Criteria, not their finer sub-criteria (e.g. CC6.1, CC6.6, CC7.2) — the same
// "verify before encoding, uncertain stays unknown" discipline compliance_pci_dss.go
// applies to PCI DSS's own sub-requirements. CC6 ("Logical and Physical Access
// Controls," which in practice covers network segmentation and encryption — the
// category most directly evidenced by an architecture diagram) is classified
// partially_assessable: this catalog runs the SAME "database not publicly routable"
// check PCI DSS Requirement 1 uses, but maps its result onto ComplianceApplicable
// rather than ComplianceSatisfied, since CC6 as a whole additionally requires
// operational evidence (access review cadence, credential rotation, offboarding)
// no architecture diagram can show — see compliance_catalog.go's own doc comment for
// why ComplianceApplicable is the right status for this. The other eight categories
// are not_assessable_from_architecture: control environment, communication, risk
// assessment, monitoring, and change-management practices are organizational
// processes, not structural facts a Terraform diagram encodes.
package core

// BuildSOC2Catalog runs every SOC 2 control this catalog declares against
// ir/workload. Category short titles are independently-paraphrased summaries of each
// Common Criteria category's own well-known public topic (see compliance_catalog.go's
// own doc comment for the citation) — never AICPA's verbatim criteria text.
func BuildSOC2Catalog(ir *IR, workload Workload) []ComplianceControlResult {
	prov := NewProvenance(KindDerived, "core/compliance_soc2")
	var results []ComplianceControlResult

	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc1", FrameworkSOC2, "CC1", "Control environment: organizational integrity and security governance", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc2", FrameworkSOC2, "CC2", "Communication and information: security policies communicated internally and externally", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc3", FrameworkSOC2, "CC3", "Risk assessment: identifying and analyzing risk to objectives", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc4", FrameworkSOC2, "CC4", "Monitoring activities: evaluating control effectiveness over time", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc5", FrameworkSOC2, "CC5", "Control activities: selecting and developing controls that mitigate risk", prov))
	results = append(results, soc2CC6LogicalAccessControls(ir, prov)...)
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc7", FrameworkSOC2, "CC7", "System operations: detecting and responding to security incidents", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc8", FrameworkSOC2, "CC8", "Change management: authorizing, testing, and approving system changes", prov))
	results = append(results, notAssessableFromArchitectureResult(
		"soc2.cc9", FrameworkSOC2, "CC9", "Risk mitigation: managing risk from vendors and business disruption", prov))

	return results
}

// soc2CC6LogicalAccessControls is CC6's own architecturally-assessable slice: the
// same "database not publicly routable" fact PCI DSS Requirement 1 checks, mapped
// onto ComplianceApplicable — CC6's own full scope (credential lifecycle, access
// reviews, physical access) needs process evidence this diagram cannot provide, so
// even a fully "satisfied" architectural finding here is reported as an
// applicability fact, never a bare control-level pass.
func soc2CC6LogicalAccessControls(ir *IR, prov Provenance) []ComplianceControlResult {
	var databases []Node
	for _, n := range ir.Nodes {
		if n.Type == NodeTypeManagedDatabase {
			databases = append(databases, n)
		}
	}
	if len(databases) == 0 {
		r := notAssessableFromArchitectureResultWithRationale(
			"soc2.cc6.no_public_database_route", FrameworkSOC2, "CC6",
			"Logical access controls: customer data store not directly reachable from the internet", prov,
			"this IR has no managed_database node to evaluate")
		r.Classification = ClassificationPartial
		return []ComplianceControlResult{r}
	}

	var out []ComplianceControlResult
	for _, db := range databases {
		r := databaseNotPubliclyRoutableResult(ir, db, "soc2.cc6.no_public_database_route", FrameworkSOC2, "CC6",
			"Logical access controls: customer data store not directly reachable from the internet", prov)
		r.Classification = ClassificationPartial
		// CC6 as a whole additionally requires operational evidence — even a
		// structurally satisfied architectural finding is only ever "applicable"
		// (in scope, architecturally supported), never a full control-level pass.
		if r.Result.Status == ComplianceSatisfied {
			r.Result.Status = ComplianceApplicable
			r.Rationale += "; the architectural aspect of CC6 is supported, but the full control additionally requires operational evidence (access review cadence, credential rotation) this diagram cannot provide"
		}
		out = append(out, r)
	}
	return out
}
