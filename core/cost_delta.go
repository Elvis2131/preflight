// This file is PC-118's Assurance Delta extension: "per-component cost increases/
// decreases, new/removed priced components, and components moving between priced
// and cost_unknown." Kept as its own dedicated type (CostDeltaEntry), not folded into
// the existing DeltaEntry/DeltaKind (core/scorecard.go) — that vocabulary is
// compliance-shaped (satisfied/unsatisfied/...) and carries no magnitude; a cost
// delta's whole point is the dollar amount that changed, which DeltaEntry has no
// field for at all.
//
// DECISION, recorded per the Card's own explicit instruction ("Decide in this ticket
// and record the decision; don't let it be implicit"): when the old and new versions
// were priced against DIFFERENT pricing snapshots, this function reports the
// price-movement component SEPARATELY rather than re-pricing either version — it
// never classifies a component as "increased"/"decreased" in that case (that
// classification asserts the change is due to design, and it cannot honestly be
// separated from AWS's own list price having moved between snapshots). Structural
// facts (a component became newly priced, became cost_unknown, was added, or was
// removed) remain valid and ARE still reported even when snapshots differ — those are
// not claims about magnitude, only about presence/knowability, which snapshot drift
// does not put in question.
package core

// CostDeltaKind names what changed for one component between two cost reports.
type CostDeltaKind string

const (
	CostDeltaIncreased        CostDeltaKind = "increased"
	CostDeltaDecreased        CostDeltaKind = "decreased"
	CostDeltaUnchanged        CostDeltaKind = "unchanged"
	CostDeltaBecamePriced     CostDeltaKind = "became_priced"                 // was cost_unknown (or absent), now priced
	CostDeltaBecameUnknown    CostDeltaKind = "became_unknown"                // was priced, now cost_unknown
	CostDeltaComponentAdded   CostDeltaKind = "component_added"               // node didn't exist in the old version at all
	CostDeltaComponentRemoved CostDeltaKind = "component_removed"             // node existed before, gone now
	CostDeltaPriceMovement    CostDeltaKind = "price_movement_not_classified" // see this file's own DECISION doc comment
)

// CostDeltaEntry is one component's cost change.
type CostDeltaEntry struct {
	NodeID           string
	Kind             CostDeltaKind
	OldMonthlyAmount float64 // 0 when the component was absent/unknown in the old version
	NewMonthlyAmount float64 // 0 when the component is absent/unknown in the new version
	ChangeAmount     float64 // NewMonthlyAmount - OldMonthlyAmount; only meaningful for Increased/Decreased
	Currency         string
	Provenance       Provenance
}

// ComputeCostDelta compares two CostReports. snapshotChanged reports whether old and
// new were priced against different snapshot IDs — the caller (a report/UI) should
// display this prominently: per-component increased/decreased entries are never
// produced in that case (this file's own DECISION), only structural ones.
//
// Either report may be nil (no pricing was available for that version at all) — this
// is handled the same way ComputeDelta/BuildScorecard already treat "not previously
// assessable": a component absent from a nil report is treated as cost_unknown/absent,
// not as priced at zero.
func ComputeCostDelta(oldReport, newReport *CostReport, prov Provenance) (entries []CostDeltaEntry, snapshotChanged bool) {
	if oldReport != nil && newReport != nil {
		snapshotChanged = oldReport.SnapshotID != newReport.SnapshotID
	}

	oldByID := componentsByID(oldReport)
	newByID := componentsByID(newReport)

	seen := map[string]bool{}
	var ids []string
	if oldReport != nil {
		for _, c := range oldReport.Components {
			if !seen[c.NodeID] {
				seen[c.NodeID] = true
				ids = append(ids, c.NodeID)
			}
		}
	}
	if newReport != nil {
		for _, c := range newReport.Components {
			if !seen[c.NodeID] {
				seen[c.NodeID] = true
				ids = append(ids, c.NodeID)
			}
		}
	}

	for _, id := range ids {
		oldC, hadOld := oldByID[id]
		newC, hasNew := newByID[id]
		entries = append(entries, classifyCostDelta(id, oldC, hadOld, newC, hasNew, snapshotChanged, prov))
	}
	return entries, snapshotChanged
}

func componentsByID(report *CostReport) map[string]ComponentCost {
	m := map[string]ComponentCost{}
	if report == nil {
		return m
	}
	for _, c := range report.Components {
		m[c.NodeID] = c
	}
	return m
}

func classifyCostDelta(nodeID string, oldC ComponentCost, hadOld bool, newC ComponentCost, hasNew bool, snapshotChanged bool, prov Provenance) CostDeltaEntry {
	entry := CostDeltaEntry{NodeID: nodeID, Currency: "USD", Provenance: prov}

	oldPriced := hadOld && oldC.Decision == CostPriced
	newPriced := hasNew && newC.Decision == CostPriced
	if oldPriced {
		entry.OldMonthlyAmount = oldC.MonthlyAmount
	}
	if newPriced {
		entry.NewMonthlyAmount = newC.MonthlyAmount
	}

	switch {
	case !hadOld && hasNew:
		entry.Kind = CostDeltaComponentAdded
	case hadOld && !hasNew:
		entry.Kind = CostDeltaComponentRemoved
	case oldPriced && !newPriced:
		entry.Kind = CostDeltaBecameUnknown
	case !oldPriced && newPriced:
		entry.Kind = CostDeltaBecamePriced
	case oldPriced && newPriced:
		entry.ChangeAmount = newC.MonthlyAmount - oldC.MonthlyAmount
		switch {
		case snapshotChanged:
			entry.Kind = CostDeltaPriceMovement
		case entry.ChangeAmount > 0:
			entry.Kind = CostDeltaIncreased
		case entry.ChangeAmount < 0:
			entry.Kind = CostDeltaDecreased
		default:
			entry.Kind = CostDeltaUnchanged
		}
	default:
		// Neither side priced (both cost_unknown, both present) — nothing changed
		// about pricing knowledge either way.
		entry.Kind = CostDeltaUnchanged
	}
	return entry
}
