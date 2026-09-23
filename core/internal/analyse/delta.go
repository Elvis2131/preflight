// This file is PC-19's core: version diffing and the Assurance Delta. CLAUDE.md §10's
// rule ("per-dimension, never a composite architecture score") extends here from a
// single finding's dimensions to the per-VERSION scorecard: a scorecard is a LIST of
// per-finding statuses, and a delta between two scorecards is a LIST of per-finding
// changes — never reduced to one number anywhere in this file.
//
// SCOPE BOUNDARY, stated rather than silently assumed: this engine diffs
// ComplianceStatus-shaped outcomes (satisfied | partial | unsatisfied | applicable |
// not_assessable) specifically, because those five values have a natural, defensible
// ordering for detecting improvement vs regression. A structural finding's free-form
// Outcome (e.g. zone-kill's "2 component(s) affected") has no such ordering — is 3
// components worse than 2? Not necessarily, it depends which components — and
// inventing one to force a comparison would be exactly the kind of fabricated
// precision CLAUDE.md §10 already warns against for severity scoring. Diffing
// structural findings meaningfully is a real, separate piece of future work, not
// something this ticket silently claims to solve by only ever testing the compliance
// case.
//
// Same package rule as everywhere else here: no dependency on preflight/core.
package analyse

import "sort"

// DeltaKind is the Assurance Delta's own per-entry classification — six values (the
// original five below, plus DeltaResolvedRisk added by PC-83), rather than collapsing
// "better/worse" into a signed number.
type DeltaKind string

const (
	DeltaImprovement   DeltaKind = "improvement"
	DeltaRegression    DeltaKind = "regression"
	DeltaNewRisk       DeltaKind = "new_risk"
	DeltaUnchanged     DeltaKind = "unchanged"
	DeltaNotAssessable DeltaKind = "not_assessable"

	// DeltaResolvedRisk (PC-83, PRD §5.7's own resolved_risks[] category) is
	// deliberately distinct from DeltaImprovement: an improvement moves a KNOWN
	// rankable state to a better one (unsatisfied -> satisfied); a resolved risk
	// closes an epistemic gap (not_assessable -> satisfied) — "we didn't know if this
	// was a problem" becoming "we now know it isn't" is a different claim from "we knew
	// it was a problem and fixed it", even though both are good news. Conflating them
	// was PC-83's own root cause everywhere this mattered (see comparedEntry).
	DeltaResolvedRisk DeltaKind = "resolved_risk"
)

// statusRank orders the three DIRECTLY comparable ComplianceStatus values from worst
// to best, for delta computation only — this is an internal ordinal used to detect the
// DIRECTION of a change between two categorical values; it is never itself surfaced as
// a score, and "applicable"/"not_assessable" are deliberately excluded (unrankable —
// comparing against an unknown state is itself unknown, not a defaulted middle value).
func statusRank(status string) (rank int, ok bool) {
	switch status {
	case "unsatisfied":
		return 0, true
	case "partial":
		return 1, true
	case "satisfied":
		return 2, true
	default:
		return 0, false
	}
}

// DeltaEntry is one finding's change between two versions.
type DeltaEntry struct {
	FindingID string
	Kind      DeltaKind
	OldStatus string // "" if the finding did not exist in the old version
	NewStatus string // "" if the finding no longer exists in the new version
	Reason    string // populated when Kind == not_assessable
}

// ComputeDelta compares two versions' finding statuses (finding ID -> ComplianceStatus
// string) and classifies every finding ID that appears in either map.
//
// PC-83's own root cause, fixed at the CALLER, not here: this function has always
// correctly distinguished "present in both, one side unrankable" from "new/removed" —
// the bug was that BuildScorecard (core/scorecard.go) used to silently EXCLUDE
// not_assessable findings from the map it hands this function, so a finding that
// existed the whole time but only became assessable in the newer version looked
// identical to a finding that never existed before. BuildScorecard now includes every
// finding with the literal status string "not_assessable" when unassessed, so this
// function sees the real "present in both" case it was always able to classify.
//
// Rules, each independently defensible rather than a single blanket "compare and
// subtract":
//   - present in both, both rankable: compare rank — higher wins (improvement),
//     lower loses (regression), equal is unchanged.
//   - present in both, old was not_assessable and new is rankable: a previously
//     UNKNOWN state resolved into a known one — ResolvedRisk if the new state is
//     satisfied (an unknown turned out fine), NewRisk if unsatisfied/partial (an
//     unknown turned out to be a real problem). Distinct from "improvement": we are not
//     claiming a rankable state got better, we are claiming an unknown got answered.
//   - present in both, new is not_assessable and old was rankable: the reverse —
//     information was LOST between versions (we used to have an answer, now we don't).
//     Always not_assessable, never a claimed direction, for the same reason a removal
//     is: we cannot say whether the underlying thing changed or just our ability to
//     assess it did.
//   - present in both, both unrankable in some other way (e.g. two different
//     non-"not_assessable" unrankable strings): not_assessable — we cannot confidently
//     say which direction an unknown moved.
//   - new in the newer version only: NewRisk if its status is unsatisfied/partial,
//     Improvement if satisfied (a brand-new good finding is a real improvement to
//     overall assurance posture), not_assessable if unrankable. (Deliberately NOT
//     ResolvedRisk — a finding that never existed before has no prior risk to resolve;
//     ResolvedRisk is reserved for a finding that existed in both versions.)
//   - present in the older version only (removed): always not_assessable — a removed
//     finding usually means the underlying resource itself was removed (e.g. golden/
//     aws-broken's defect 4 removes the DLQ entirely), and claiming a direction for
//     "we no longer check this" would be guessing, not deriving.
func ComputeDelta(oldStatuses, newStatuses map[string]string) []DeltaEntry {
	seen := map[string]bool{}
	var entries []DeltaEntry

	for id := range oldStatuses {
		seen[id] = true
	}
	for id := range newStatuses {
		seen[id] = true
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids) // determinism (I1) — same discipline as every other engine here

	for _, id := range ids {
		oldStatus, hadOld := oldStatuses[id]
		newStatus, hasNew := newStatuses[id]

		switch {
		case hadOld && !hasNew:
			entries = append(entries, DeltaEntry{
				FindingID: id, Kind: DeltaNotAssessable, OldStatus: oldStatus,
				Reason: "finding was removed between versions — the underlying resource may have been removed too; claiming a direction would be guessing, not deriving",
			})
		case !hadOld && hasNew:
			entries = append(entries, newFindingEntry(id, newStatus))
		default: // present in both
			entries = append(entries, comparedEntry(id, oldStatus, newStatus))
		}
	}
	return entries
}

func newFindingEntry(id, newStatus string) DeltaEntry {
	rank, ok := statusRank(newStatus)
	if !ok {
		return DeltaEntry{FindingID: id, Kind: DeltaNotAssessable, NewStatus: newStatus,
			Reason: "new finding's status (\"" + newStatus + "\") is not directly rankable"}
	}
	if rank == 2 { // satisfied
		return DeltaEntry{FindingID: id, Kind: DeltaImprovement, NewStatus: newStatus}
	}
	return DeltaEntry{FindingID: id, Kind: DeltaNewRisk, NewStatus: newStatus}
}

func comparedEntry(id, oldStatus, newStatus string) DeltaEntry {
	oldRank, oldOK := statusRank(oldStatus)
	newRank, newOK := statusRank(newStatus)

	// An unknown resolving into a known state is a different claim from a known state
	// moving along its own scale — see ComputeDelta's own doc comment. Checked before
	// the generic "either side unrankable" fallback so this specific, common case gets
	// its own honest classification instead of collapsing into a blanket unknown.
	if oldStatus == "not_assessable" && newOK {
		if newRank == 2 { // satisfied
			return DeltaEntry{FindingID: id, Kind: DeltaResolvedRisk, OldStatus: oldStatus, NewStatus: newStatus}
		}
		return DeltaEntry{FindingID: id, Kind: DeltaNewRisk, OldStatus: oldStatus, NewStatus: newStatus}
	}
	if newStatus == "not_assessable" && oldOK {
		return DeltaEntry{FindingID: id, Kind: DeltaNotAssessable, OldStatus: oldStatus, NewStatus: newStatus,
			Reason: "this finding was assessable in the older version but is not_assessable in the newer one — information was lost, not necessarily the underlying state; claiming a direction would be guessing"}
	}

	if !oldOK || !newOK {
		return DeltaEntry{FindingID: id, Kind: DeltaNotAssessable, OldStatus: oldStatus, NewStatus: newStatus,
			Reason: "at least one side's status is not directly rankable (applicable/not_assessable)"}
	}
	switch {
	case newRank > oldRank:
		return DeltaEntry{FindingID: id, Kind: DeltaImprovement, OldStatus: oldStatus, NewStatus: newStatus}
	case newRank < oldRank:
		return DeltaEntry{FindingID: id, Kind: DeltaRegression, OldStatus: oldStatus, NewStatus: newStatus}
	default:
		return DeltaEntry{FindingID: id, Kind: DeltaUnchanged, OldStatus: oldStatus, NewStatus: newStatus}
	}
}
