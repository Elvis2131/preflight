// This file is PC-121's own registry infrastructure: a real control catalog
// (framework, requirement ID, paraphrased title, and an honest classification of how
// much a static architecture diagram can actually evidence) that PC-110's future
// framework selector can query without ever offering a framework this engine cannot
// assess (the Card's own explicit requirement).
//
// Classification, verbatim from the Card:
//   - assessable_from_architecture: this engine has a real, deterministic check.
//   - partially_assessable: architecture supports it, but process/operational
//     evidence beyond what a Terraform diagram can show is also required. Its
//     architectural finding maps onto ComplianceApplicable (core/compliance.go's own
//     five-value vocabulary) rather than ComplianceSatisfied — this is the first
//     real consumer of that value's own previously-stated ambiguity (PC-18's doc
//     comment on ComplianceApplicable posed two readings; this ticket resolves it in
//     favor of reading (b), "a control-scope-level fact distinct from a per-resource
//     verdict": partial architectural support IS exactly that fact).
//   - not_assessable_from_architecture: no architecture diagram can ever evidence
//     this control (people, process, physical security, organizational policy).
//     These entries carry NO evaluation logic at all — their result is always
//     ComplianceNotAssessable, by construction, never computed and never capable of
//     silently becoming "satisfied."
//
// Licensing, per the Card's own explicit instruction: only requirement/criterion IDs and short,
// independently-written titles are stored, never the standards' text. The PCI DSS v4.0.1 and AICPA
// 2017 Trust Services Criteria (revised points of focus, 2022) catalogs were built by reading both
// standards locally (the source PDFs are git-ignored); compliance_pci_dss.go and compliance_soc2.go each
// state their own scope, and docs/COMPLIANCE_CATALOG.md summarises it. The shared checks behind the
// assessable and partially assessable controls are in compliance_catalog_checks.go.
package core

// ComplianceFramework is one selectable compliance framework — PC-110's future
// framework selector reads exactly this catalog to decide what it may offer.
type ComplianceFramework string

const (
	FrameworkPCIDSS4 ComplianceFramework = "pci_dss_4"
	FrameworkSOC2    ComplianceFramework = "soc2"
	FrameworkCISAWS  ComplianceFramework = "cis_aws"
)

// ControlClassification is the Card's own three-value vocabulary — see this file's
// own doc comment for the full meaning of each.
type ControlClassification string

const (
	ClassificationAssessable    ControlClassification = "assessable_from_architecture"
	ClassificationPartial       ControlClassification = "partially_assessable"
	ClassificationNotAssessable ControlClassification = "not_assessable_from_architecture"
)

// ComplianceControlResult is one control's result — for a NotAssessable-classified
// control this is always exactly one entry, Status forced to ComplianceNotAssessable;
// for an Assessable/Partial control it may be one entry per resource the control
// actually evaluated (e.g. one per managed_database node), or a single
// not_assessable entry when the IR contains no resource this control applies to at
// all (never silently omitted — "12/12 satisfied" must never hide an inapplicable
// control by leaving it out of the count).
type ComplianceControlResult struct {
	ControlID      string
	Framework      ComplianceFramework
	RequirementID  string
	Title          string
	Classification ControlClassification
	NodeID         string // empty when this result is not about one specific resource
	Result         ComplianceResult
	Rationale      string
}

// notAssessableFromArchitectureResult builds the one, structurally-guaranteed result
// a NotAssessable-classified control ever produces — there is no Evaluate function to
// call, and so no code path through which this could ever become "satisfied."
func notAssessableFromArchitectureResult(controlID string, framework ComplianceFramework, requirementID, title string, prov Provenance) ComplianceControlResult {
	reason := "this control concerns people, process, physical security, or organizational policy — no architecture diagram can evidence it"
	return ComplianceControlResult{
		ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
		Classification: ClassificationNotAssessable,
		Result:         ComplianceResult{Status: ComplianceNotAssessable, Provenance: prov},
		Rationale:      reason,
	}
}

// notAssessableFromArchitectureResultWithRationale is
// notAssessableFromArchitectureResult with a specific, real reason (e.g. "this IR has
// no managed_database node to evaluate") instead of the generic people/process/
// physical-security reason — used when a control IS architecturally assessable in
// principle but this particular IR has no resource it applies to at all. Still always
// ComplianceNotAssessable, never silently omitted from the count.
func notAssessableFromArchitectureResultWithRationale(controlID string, framework ComplianceFramework, requirementID, title string, prov Provenance, rationale string) ComplianceControlResult {
	return ComplianceControlResult{
		ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
		Classification: ClassificationAssessable,
		Result:         ComplianceResult{Status: ComplianceNotAssessable, Provenance: prov},
		Rationale:      rationale,
	}
}

// databaseNotPubliclyRoutableResult is shared by PCI DSS Requirement 1 and SOC 2 CC6
// (both catalogs independently classify "is the cardholder/customer data store
// reachable from the internet" as architecturally assessable) — reuses
// core.IsPublicSubnet (core/routing.go, PC-111) verbatim, zero new evaluation logic.
func databaseNotPubliclyRoutableResult(ir *IR, db Node, controlID string, framework ComplianceFramework, requirementID, title string, prov Provenance) ComplianceControlResult {
	nodeID := db.ID
	subnetID, hasSubnet := resolveSubnetID(ir, db.ID)
	if !hasSubnet {
		return ComplianceControlResult{
			ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
			Classification: ClassificationAssessable, NodeID: nodeID,
			Result:    ComplianceResult{Status: ComplianceNotAssessable, Provenance: prov},
			Rationale: "this resource has no resolvable subnet placement",
		}
	}
	isPublic, hasRouteTable := IsPublicSubnetIR(ir, subnetID)
	if !hasRouteTable {
		return ComplianceControlResult{
			ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
			Classification: ClassificationAssessable, NodeID: nodeID,
			Result:    ComplianceResult{Status: ComplianceNotAssessable, Provenance: prov},
			Rationale: "this resource's own subnet has no resolvable effective route table",
		}
	}
	if isPublic {
		return ComplianceControlResult{
			ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
			Classification: ClassificationAssessable, NodeID: nodeID,
			Result:    ComplianceResult{Status: ComplianceUnsatisfied, Provenance: prov},
			Rationale: "this resource's own subnet has a route to an Internet Gateway — it is directly reachable from the internet",
		}
	}
	return ComplianceControlResult{
		ControlID: controlID, Framework: framework, RequirementID: requirementID, Title: title,
		Classification: ClassificationAssessable, NodeID: nodeID,
		Result:    ComplianceResult{Status: ComplianceSatisfied, Provenance: prov},
		Rationale: "this resource's own subnet has no route to an Internet Gateway",
	}
}

// ComplianceCatalogCounts is PC-121's own acceptance criterion, verbatim: "Report
// shows counts for all three classes." Counts by CLASSIFICATION (how many controls
// this catalog declares in each bucket), independent of any one IR's own results —
// PC-110's framework selector needs this to show "this framework has N
// assessable / M partial / K not-assessable controls" before any architecture is
// even loaded.
type ComplianceCatalogCounts struct {
	Assessable    int
	Partial       int
	NotAssessable int
}

// ComplianceResultCounts tallies actual per-run STATUS outcomes (core.ComplianceStatus's
// own five values) across a set of ComplianceControlResult — the other half of
// "counts for all three classes": once results exist, how many came back in each
// status. Applicable is the status a Partial-classified control's own architectural
// finding maps onto (never Satisfied) — see this file's own doc comment. A
// not_assessable-classified control's own result is, by construction
// (notAssessableFromArchitectureResult), always ComplianceNotAssessable — this
// function does not special-case that; it is simply never able to produce anything
// else, so the tally is honest by construction, not by a check performed here.
type ComplianceResultCounts struct {
	Satisfied     int
	Applicable    int
	Partial       int
	Unsatisfied   int
	NotAssessable int
}

// SummarizeComplianceCatalogCounts tallies by CLASSIFICATION (never by status) — "how
// many of this framework's own controls are assessable/partial/not-assessable,"
// independent of any per-control result. This is what "nobody reads 12/12 satisfied
// when 12 is only the architectural slice" (the Card's own words) needs shown
// alongside the satisfied count: the denominator context.
func SummarizeComplianceCatalogCounts(results []ComplianceControlResult) ComplianceCatalogCounts {
	seen := map[string]bool{}
	var c ComplianceCatalogCounts
	for _, r := range results {
		if seen[r.ControlID] {
			continue // one control may produce several per-resource results; counted once
		}
		seen[r.ControlID] = true
		switch r.Classification {
		case ClassificationAssessable:
			c.Assessable++
		case ClassificationPartial:
			c.Partial++
		case ClassificationNotAssessable:
			c.NotAssessable++
		}
	}
	return c
}

func SummarizeComplianceResults(results []ComplianceControlResult) ComplianceResultCounts {
	var c ComplianceResultCounts
	for _, r := range results {
		switch r.Result.Status {
		case ComplianceSatisfied:
			c.Satisfied++
		case ComplianceApplicable:
			c.Applicable++
		case CompliancePartial:
			c.Partial++
		case ComplianceUnsatisfied:
			c.Unsatisfied++
		case ComplianceNotAssessable:
			c.NotAssessable++
		}
	}
	return c
}
