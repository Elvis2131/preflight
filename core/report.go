// This file is PC-120: a single structured report for a stored design version.
// PROJECTION ONLY, per the Card's own explicit instruction — every value here is
// produced by an EXISTING engine (BuildFindings, the PC-121 compliance catalogs,
// ComputeAllJourneyFlows/ComputeComponentLoad, ComputeCost, ComputeDelta), called
// again against the same already-stored IR/Workload/Cost a version was assessed
// with. This file adds no verdict, scoring, or simulation logic of its own — if a
// reviewer asks "where did this verdict come from," the answer is always one of
// those engines' own IDs, never report code.
package core

import (
	"reflect"
	"sort"
)

// ReportInventoryEntry is one IR node's own compact inventory listing — ID, type,
// and resolution only, deliberately NOT the node's full Provenance/Capability/
// RawAttributes (see ReportAssumptions's own doc comment for why: embedding every
// node's own "stated, source: <its own Terraform address>" provenance here would
// flood the assumptions appendix with entries that restate this same inventory
// section, not surface the inputs a reviewer should actually question).
type ReportInventoryEntry struct {
	NodeID     string          `json:"node_id"`
	Type       NodeType        `json:"type"`
	Resolution ResolutionState `json:"resolution"`
}

// ReportNFREntry maps one declared Requirement to the findings that evaluate it —
// the Card's own instruction, verbatim: "Requirements with no finding that addresses
// them are listed as 'not evaluated.'"
type ReportNFREntry struct {
	RequirementID string              `json:"requirement_id"`
	Priority      RequirementPriority `json:"priority"`
	Value         any                 `json:"value"`
	Evaluated     bool                `json:"evaluated"`
	FindingIDs    []string            `json:"finding_ids,omitempty"`
	Reason        string              `json:"reason,omitempty"`
}

// ReportComplianceFrameworkSection is one framework's full catalog run — PC-121's own
// BuildPCIDSS4Catalog/BuildSOC2Catalog/BuildCISAWSCatalog output, plus both count
// summaries the Card's own acceptance criterion asks the report to show ("counts for
// all three classes").
type ReportComplianceFrameworkSection struct {
	Framework     ComplianceFramework       `json:"framework"`
	CatalogCounts ComplianceCatalogCounts   `json:"catalog_counts"`
	ResultCounts  ComplianceResultCounts    `json:"result_counts"`
	Controls      []ComplianceControlResult `json:"controls"`
}

// ReportFailureModesSection wraps BuildFindings' own output. Available is always
// true today (BuildFindings always runs); the field exists for the same
// "unbuilt-engine sections render explicitly" shape every other section uses, so a
// future genuinely-unbuilt failure-mode class has somewhere to report through
// without changing this type.
type ReportFailureModesSection struct {
	Available         bool      `json:"available"`
	UnavailableReason string    `json:"unavailable_reason,omitempty"`
	Findings          []Finding `json:"findings,omitempty"`
}

// ReportTrafficSection wraps PC-125/126's own ComputeAllJourneyFlows/
// ComputeComponentLoad — unavailable (not "empty") only when the workload declares no
// journeys at all, since a real journey list producing zero flows is a genuine
// result, not an absence.
type ReportTrafficSection struct {
	Available         bool                `json:"available"`
	UnavailableReason string              `json:"unavailable_reason,omitempty"`
	Flows             []JourneyFlowResult `json:"flows,omitempty"`
	Load              []ComponentLoad     `json:"load,omitempty"`
}

// ReportCostSection wraps PC-117's own CostReport. Unavailable when no pricing
// snapshot was attached at assessment time (Cost == nil) — a real, already-existing
// case (server.Store with no pricing store attached), not a fabricated placeholder.
type ReportCostSection struct {
	Available         bool        `json:"available"`
	UnavailableReason string      `json:"unavailable_reason,omitempty"`
	Disclaimer        string      `json:"disclaimer,omitempty"`
	Report            *CostReport `json:"report,omitempty"`

	// UsageBasedCharges is PC-132's own addition — NAT data processing, internet
	// egress, cross-AZ transfer, and LB data-processed LCU charges computed from
	// declared traffic (ComputeUsageBasedCost). Always present (never null, even
	// empty — no journeys declared, or no price table was available, both real,
	// distinct-from-each-other reasons a caller can already see elsewhere in this
	// same report, so no separate unavailable_reason is duplicated here).
	UsageBasedCharges []UsageBasedCostEntry `json:"usage_based_charges"`
}

// costDisclaimer is the Card's own required "cost with disclaimer" section text —
// stated once here, not duplicated at each call site.
const costDisclaimer = "Cost figures are estimates derived from a dated AWS Price List snapshot and declared sizing. They are not a bill, do not reflect any negotiated discount, and do not include usage-based charges (data transfer, request volume, storage I/O) unless separately modelled."

// ReportExecutiveSummary is per-dimension counts only — CLAUDE.md §10's own rule
// ("per-dimension, never a composite architecture score") applied to the report as a
// whole, the same discipline core.Scorecard's own structural test already enforces
// for a single version.
type ReportExecutiveSummary struct {
	ScorecardStatusCounts   map[string]int                                  `json:"scorecard_status_counts"`
	ComplianceCatalogCounts map[ComplianceFramework]ComplianceCatalogCounts `json:"compliance_catalog_counts"`
	ComplianceResultCounts  map[ComplianceFramework]ComplianceResultCounts  `json:"compliance_result_counts"`
	NFREvaluatedCount       int                                             `json:"nfr_evaluated_count"`
	NFRNotEvaluatedCount    int                                             `json:"nfr_not_evaluated_count"`
}

// ReportAssumption is one provenance-tagged value the underlying assessment either
// assumed (a declared default the architect can override) or the architect
// themselves stated — "the concrete payoff of I2," the Card's own words.
//
// SCOPE, stated explicitly: this walks the report's own computed sections (failure
// modes, traffic, compliance, cost, NFR conformance, delta, executive summary) —
// deliberately NOT the Inventory section's own per-node Provenance. Every real IR
// node's own Provenance is Kind=stated (ingest/build.go tags every node "from its own
// Terraform address"), so including Inventory here would flood this appendix with
// one entry per resource in the bundle (35+ for golden/aws alone) — technically "a
// stated value," but not what an assumptions appendix is FOR (surfacing inputs and
// overridable defaults a reviewer should question, not restating the inventory
// section that already lists every resource by name). A second real discovery, made
// while building this: Kind=assumed is not actually produced ANYWHERE in this
// codebase today (grep confirms it — ingest/build.go's own buildCapability doc
// comment already records the reason: a per-field assumed-default mechanism was
// deliberately deferred, a real, already-documented, pre-existing gap, not something
// this ticket introduces or hides). So today's real appendix, for any version, is
// either empty or contains only "stated" entries from whichever computed sections
// happen to carry one — an honest, verifiable fact, not a placeholder.
type ReportAssumption struct {
	Kind   Kind   `json:"kind"`
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

// Report is PC-120's full contract type — report.schema.json's own root.
type Report struct {
	SessionID     string `json:"session_id" validate:"required" jsonschema:"required,minLength=1"`
	VersionNumber int    `json:"version_number" validate:"gte=1" jsonschema:"required,minimum=1"`
	// Graph is PC-122's own addition — the PC-81 SVG diagram, embedded so the HTML/
	// PDF renderer never re-lays it out (the Card's own explicit instruction: "Embeds
	// the PC-81 SVG diagram directly"). Always populated with either the real SVG or
	// a real, specific failure message (never a blank string standing in for one) —
	// the exact same "never fabricate a diagram" discipline AssessResponse.Graph
	// already established (server/assess.go's graphOrFailureMessage). Rendering
	// itself needs subprocess I/O (render.SVG), which core/ can never do (I1) —
	// BuildReport takes the already-rendered string from its own caller, the same
	// "pure engine, I/O caller supplies the one fact it can't compute itself" split
	// PC-115/117's own Sizing/PriceTable parameters already use.
	Graph string `json:"graph" validate:"required" jsonschema:"required,minLength=1"`

	ExecutiveSummary ReportExecutiveSummary             `json:"executive_summary" validate:"required" jsonschema:"required"`
	Inventory        []ReportInventoryEntry             `json:"inventory"`
	NFRConformance   []ReportNFREntry                   `json:"nfr_conformance"`
	Compliance       []ReportComplianceFrameworkSection `json:"compliance"`
	FailureModes     ReportFailureModesSection          `json:"failure_modes" validate:"required" jsonschema:"required"`
	Traffic          ReportTrafficSection               `json:"traffic" validate:"required" jsonschema:"required"`
	Cost             ReportCostSection                  `json:"cost" validate:"required" jsonschema:"required"`
	Delta            []DeltaEntry                       `json:"delta"`
	// Scenarios is PC-131's own addition: the session's saved Failure Lab scenarios,
	// each evaluated NOW against THIS report's version (never replayed from an earlier
	// result). Empty (never null) when none are saved. Filled by the caller that can
	// read saved definitions (server.GetReport) via Report.WithScenarios — BuildReport
	// itself stays a pure function of the stored version.
	Scenarios   []ScenarioResult   `json:"scenarios"`
	Assumptions []ReportAssumption `json:"assumptions"`
	Provenance       Provenance                         `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this Report against the same struct tags contracts/
// report.schema.json is generated from.
func (r Report) Validate() error {
	return validate.Struct(r)
}

// WithScenarios returns r with its saved scenarios evaluated against r's own version.
func (r Report) WithScenarios(ir *IR, workload Workload, saved []SavedScenario) Report {
	prov := NewProvenance(KindDerived, "core/report:scenarios")
	r.Scenarios = EvaluateScenarios(ir, workload, saved, prov)
	return r
}

// BuildReport assembles PC-120's report from an already-assessed version's own
// stored facts — ir/workload/findings/scorecard/cost are exactly what StoreVersion
// (server/store.go) persisted for this version; delta is exactly what
// GetStoredVersion already computed against the prior version, reused verbatim, not
// recomputed here.
func BuildReport(sessionID string, versionNumber int, graphSVG string, ir *IR, workload Workload, findings []Finding, scorecard Scorecard, cost *CostReport, priceTable PriceTable, delta []DeltaEntry) Report {
	prov := NewProvenance(KindDerived, "core/report")

	// No null in JSON (this codebase's established discipline — see e.g.
	// SimulateResponse's own FlowDetail/SeveredPaths/Cascade doc comments): a nil
	// input here means "no delta entries" (e.g. this is version 1), a real, empty
	// fact, not an absent field.
	if delta == nil {
		delta = []DeltaEntry{}
	}
	if findings == nil {
		findings = []Finding{}
	}

	inventory := make([]ReportInventoryEntry, 0, len(ir.Nodes))
	for _, n := range ir.Nodes {
		inventory = append(inventory, ReportInventoryEntry{NodeID: n.ID, Type: n.Type, Resolution: n.Resolution})
	}

	nfr := buildNFRConformance(workload.Requirements, findings)

	pciResults := BuildPCIDSS4Catalog(ir, workload)
	soc2Results := BuildSOC2Catalog(ir, workload)
	cisResults := BuildCISAWSCatalog(ir, workload)
	compliance := []ReportComplianceFrameworkSection{
		{Framework: FrameworkPCIDSS4, CatalogCounts: SummarizeComplianceCatalogCounts(pciResults), ResultCounts: SummarizeComplianceResults(pciResults), Controls: pciResults},
		{Framework: FrameworkSOC2, CatalogCounts: SummarizeComplianceCatalogCounts(soc2Results), ResultCounts: SummarizeComplianceResults(soc2Results), Controls: soc2Results},
		{Framework: FrameworkCISAWS, CatalogCounts: SummarizeComplianceCatalogCounts(cisResults), ResultCounts: SummarizeComplianceResults(cisResults), Controls: cisResults},
	}

	failureModes := ReportFailureModesSection{Available: true, Findings: findings}

	traffic := ReportTrafficSection{}
	if len(workload.Journeys) == 0 {
		traffic.UnavailableReason = "no journeys are declared in this version's workload — traffic and capacity have nothing to compute over"
	} else {
		traffic.Available = true
		traffic.Flows = ComputeAllJourneyFlows(ir, workload, nil)
		traffic.Load = RankBottlenecks(ComputeComponentLoad(ir, workload, nil))
	}

	costSection := ReportCostSection{Disclaimer: costDisclaimer, UsageBasedCharges: []UsageBasedCostEntry{}}
	if cost == nil {
		costSection.UnavailableReason = "no pricing snapshot was available when this version was assessed"
	} else {
		costSection.Available = true
		costSection.Report = cost
		if len(workload.Journeys) > 0 && priceTable.SnapshotID != "" {
			costSection.UsageBasedCharges = ComputeUsageBasedCost(ir, workload, priceTable, nil)
		}
	}

	scorecardCounts := map[string]int{}
	for _, e := range scorecard.Entries {
		scorecardCounts[e.Status]++
	}

	nfrEvaluated, nfrNotEvaluated := 0, 0
	for _, e := range nfr {
		if e.Evaluated {
			nfrEvaluated++
		} else {
			nfrNotEvaluated++
		}
	}

	summary := ReportExecutiveSummary{
		ScorecardStatusCounts: scorecardCounts,
		ComplianceCatalogCounts: map[ComplianceFramework]ComplianceCatalogCounts{
			FrameworkPCIDSS4: compliance[0].CatalogCounts, FrameworkSOC2: compliance[1].CatalogCounts, FrameworkCISAWS: compliance[2].CatalogCounts,
		},
		ComplianceResultCounts: map[ComplianceFramework]ComplianceResultCounts{
			FrameworkPCIDSS4: compliance[0].ResultCounts, FrameworkSOC2: compliance[1].ResultCounts, FrameworkCISAWS: compliance[2].ResultCounts,
		},
		NFREvaluatedCount: nfrEvaluated, NFRNotEvaluatedCount: nfrNotEvaluated,
	}

	report := Report{
		SessionID: sessionID, VersionNumber: versionNumber, Graph: graphSVG,
		ExecutiveSummary: summary, Inventory: inventory, NFRConformance: nfr, Compliance: compliance,
		FailureModes: failureModes, Traffic: traffic, Cost: costSection, Delta: delta,
		Scenarios:  []ScenarioResult{},
		Provenance: prov,
	}
	report.Assumptions = gatherReportAssumptions(report)
	return report
}

// buildNFRConformance maps each declared Requirement onto the findings that address
// it. Only two requirement IDs have a real, wired evaluation today:
// rpo_seconds (rpoFeasibilityFinding's own RPOFeasible dimension, PC-14) and
// CostBudgetRequirementID (BuildCostBudgetFindings' own fixed ID, PC-118). Every
// other requirement ID (e.g. availability.target, rto_seconds in golden/workload.yaml)
// has no engine wired to it at all yet — honestly reported "not evaluated," which the
// Card's own words call "itself useful information for the architect," not a gap to
// paper over with a guessed evaluation.
func buildNFRConformance(requirements []Requirement, findings []Finding) []ReportNFREntry {
	out := make([]ReportNFREntry, 0, len(requirements))
	for _, req := range requirements {
		var addressing []string
		for _, f := range findings {
			if requirementAddressedBy(req.ID, f) {
				addressing = append(addressing, f.ID)
			}
		}
		entry := ReportNFREntry{RequirementID: req.ID, Priority: req.Priority, Value: req.Value, FindingIDs: addressing}
		if len(addressing) == 0 {
			entry.Reason = "no finding in this version currently evaluates this requirement"
		} else {
			entry.Evaluated = true
		}
		out = append(out, entry)
	}
	return out
}

func requirementAddressedBy(requirementID string, f Finding) bool {
	switch requirementID {
	case "rpo_seconds":
		return f.Dimensions.Recoverability.RPOFeasible.State == AssessmentStateAssessed
	case CostBudgetRequirementID:
		return f.ID == "finding.cost.budget-compliance"
	default:
		return false
	}
}

// gatherReportAssumptions walks report's own computed sections (never Inventory —
// see ReportAssumption's own doc comment for why) collecting every distinct
// (Kind, Source) Provenance pair whose Kind is assumed or stated, sorted
// deterministically (NFR-1) regardless of any map traversed along the way.
func gatherReportAssumptions(report Report) []ReportAssumption {
	walkable := struct {
		ExecutiveSummary ReportExecutiveSummary
		NFRConformance   []ReportNFREntry
		Compliance       []ReportComplianceFrameworkSection
		FailureModes     ReportFailureModesSection
		Traffic          ReportTrafficSection
		Cost             ReportCostSection
		Delta            []DeltaEntry
	}{report.ExecutiveSummary, report.NFRConformance, report.Compliance, report.FailureModes, report.Traffic, report.Cost, report.Delta}

	seen := map[string]bool{}
	out := []ReportAssumption{} // no null in JSON — an empty appendix is a real, honest result
	walkForAssumedOrStatedProvenance(reflect.ValueOf(walkable), &out, seen)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Source < out[j].Source
	})
	return out
}

var provenanceType = reflect.TypeOf(Provenance{})

func walkForAssumedOrStatedProvenance(v reflect.Value, out *[]ReportAssumption, seen map[string]bool) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return
		}
		walkForAssumedOrStatedProvenance(v.Elem(), out, seen)
	case reflect.Struct:
		if v.Type() == provenanceType {
			p := v.Interface().(Provenance)
			if p.Kind == KindAssumed || p.Kind == KindStated {
				key := string(p.Kind) + "|" + p.Source
				if !seen[key] {
					seen[key] = true
					*out = append(*out, ReportAssumption{Kind: p.Kind, Source: p.Source, Reason: p.Reason})
				}
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				continue // unexported field
			}
			walkForAssumedOrStatedProvenance(v.Field(i), out, seen)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkForAssumedOrStatedProvenance(v.Index(i), out, seen)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkForAssumedOrStatedProvenance(v.MapIndex(k), out, seen)
		}
	}
}
