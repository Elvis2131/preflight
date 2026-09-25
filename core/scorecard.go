package core

import (
	"time"

	"preflight/core/internal/analyse"
)

// ScorecardEntry is one control/finding's status within a single version's scorecard.
type ScorecardEntry struct {
	FindingID  string     `json:"finding_id" validate:"required" jsonschema:"required"`
	Status     string     `json:"status" validate:"required" jsonschema:"required,description=A ComplianceStatus value for a compliance-shaped finding; this field stays a plain string rather than the ComplianceStatus type so a future non-compliance scorecard entry isn't forced into that vocabulary."`
	Provenance Provenance `json:"provenance" validate:"required" jsonschema:"required"`
}

// Scorecard is a version's full set of tracked findings' statuses — deliberately a
// LIST, never reducible to one number. CLAUDE.md §10's rule extended from a single
// finding's four dimensions to the whole per-version scorecard: "per-dimension, never
// a composite architecture score." PC-19's own acceptance criterion — "no composite
// score field exists anywhere in the scorecard schema" — is checked structurally via
// reflection in core/scorecard_test.go, the same technique established for
// Assessment[T]'s "no pass/fail field" and FailureMode's "no severity number"
// guarantees.
type Scorecard struct {
	VersionNumber int              `json:"version_number" validate:"gte=1" jsonschema:"required,minimum=1"`
	Entries       []ScorecardEntry `json:"entries" validate:"dive"`
}

// Validate checks this Scorecard against the same struct tags it is generated from.
func (s Scorecard) Validate() error {
	return validate.Struct(s)
}

// DeltaKind mirrors core/internal/analyse's own type exactly.
type DeltaKind = analyse.DeltaKind

const (
	DeltaImprovement   = analyse.DeltaImprovement
	DeltaRegression    = analyse.DeltaRegression
	DeltaNewRisk       = analyse.DeltaNewRisk
	DeltaUnchanged     = analyse.DeltaUnchanged
	DeltaNotAssessable = analyse.DeltaNotAssessable
	DeltaResolvedRisk  = analyse.DeltaResolvedRisk
)

// DeltaEntry is one finding's change between two versions — the Assurance Delta's own
// unit. Every entry carries its own Provenance (PC-19's own acceptance criterion,
// verbatim) — a delta is itself a derived fact ("this finding got better/worse"), and
// I2 applies to it exactly as it applies to any other assertion this codebase makes.
type DeltaEntry struct {
	FindingID  string     `json:"finding_id" validate:"required" jsonschema:"required"`
	Kind       DeltaKind  `json:"kind" validate:"required,oneof=improvement regression new_risk unchanged not_assessable resolved_risk" jsonschema:"required"`
	OldStatus  string     `json:"old_status,omitempty" jsonschema:"description=Empty if the finding did not exist in the old version."`
	NewStatus  string     `json:"new_status,omitempty" jsonschema:"description=Empty if the finding no longer exists in the new version."`
	Reason     string     `json:"reason,omitempty" jsonschema:"description=Populated when Kind is not_assessable."`
	Provenance Provenance `json:"provenance" validate:"required" jsonschema:"required"`
}

// Validate checks this DeltaEntry against the same struct tags it is generated from.
func (d DeltaEntry) Validate() error {
	return validate.Struct(d)
}

// ComputeDelta wraps core/internal/analyse.ComputeDelta's plain-string-keyed result
// into real DeltaEntry values with real Provenance attached to every entry — see
// ComputeDelta's own doc comment (in core/internal/analyse/delta.go) for the full
// classification rules and the scope boundary (compliance-shaped statuses only, not
// free-form structural outcomes).
func ComputeDelta(oldStatuses, newStatuses map[string]string, prov Provenance) []DeltaEntry {
	raw := analyse.ComputeDelta(oldStatuses, newStatuses)
	entries := make([]DeltaEntry, len(raw))
	for i, r := range raw {
		entries[i] = DeltaEntry{
			FindingID:  r.FindingID,
			Kind:       r.Kind,
			OldStatus:  r.OldStatus,
			NewStatus:  r.NewStatus,
			Reason:     r.Reason,
			Provenance: prov,
		}
	}
	return entries
}

// BuildScorecard extracts a version's Scorecard from a list of Findings — EVERY
// finding becomes a ScorecardEntry, using its Outcome.Value string when assessed, or
// the literal string "not_assessable" when not. This is PC-83's own root-cause fix:
// an earlier version of this function silently EXCLUDED not_assessable findings from
// the map entirely, which meant ComputeDelta (core/internal/analyse/delta.go) could
// never see "this finding existed all along but only became assessable in the newer
// version" as the distinct case it actually is — it looked identical to a finding that
// never existed before, and got the wrong classification (improvement instead of
// resolved_risk) as a result. Scorecard and delta now read finding state through this
// one function — the single shared source of truth PC-83 asked for, not two
// independently-computed paths that could drift again.
func BuildScorecard(findings []Finding, versionNumber int) Scorecard {
	sc := Scorecard{VersionNumber: versionNumber}
	for _, f := range findings {
		status, ok := f.Outcome.Value.(string)
		if !ok {
			status = "not_assessable"
		}
		sc.Entries = append(sc.Entries, ScorecardEntry{
			FindingID:  f.ID,
			Status:     status,
			Provenance: f.Outcome.Provenance,
		})
	}
	return sc
}

// ApplyWaivers is PC-20's second acceptance criterion, verbatim: "a waived finding
// displays as accepted in the scorecard, not hidden entirely." Entries whose finding
// has an applicable waiver (Waiver.Applies) get Status rewritten to "accepted" — every
// entry stays present either way, only Status changes, so a waived finding is never
// dropped from the list the way BuildScorecard's own PC-83 fix already refused to drop
// not_assessable findings. Runs as a separate pass over an already-built Scorecard,
// not folded into BuildScorecard itself, since BuildScorecard's job (finding -> raw
// status) and this one (raw status -> displayed status, given accepted risk) are
// genuinely different questions — the first is a plain projection, the second needs
// waivers and a clock BuildScorecard's callers don't always have in hand.
func ApplyWaivers(sc Scorecard, waivers []Waiver, now time.Time) Scorecard {
	applicable := make(map[string]bool, len(waivers))
	for _, w := range waivers {
		if w.Applies(sc.VersionNumber, now) {
			applicable[w.FindingID] = true
		}
	}

	out := Scorecard{VersionNumber: sc.VersionNumber, Entries: make([]ScorecardEntry, len(sc.Entries))}
	for i, e := range sc.Entries {
		if applicable[e.FindingID] {
			e.Status = "accepted"
		}
		out.Entries[i] = e
	}
	return out
}

// StatusMap extracts a plain finding-ID -> status map from a Scorecard, the shape
// ComputeDelta actually consumes.
func (s Scorecard) StatusMap() map[string]string {
	m := make(map[string]string, len(s.Entries))
	for _, e := range s.Entries {
		m[e.FindingID] = e.Status
	}
	return m
}
