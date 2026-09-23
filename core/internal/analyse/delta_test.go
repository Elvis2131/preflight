package analyse

import "testing"

// This file hand-verifies ComputeDelta against small, constructed scorecards with a
// hand-worked expected classification for every entry — before this touches any real
// golden-fixture data — the same sequencing discipline mincut.go's own tests
// established: prove the algorithm right on a case small enough to check by eye first.

func findEntry(t *testing.T, entries []DeltaEntry, id string) DeltaEntry {
	t.Helper()
	for _, e := range entries {
		if e.FindingID == id {
			return e
		}
	}
	t.Fatalf("no delta entry for finding %q", id)
	return DeltaEntry{}
}

// TestComputeDelta_AllFiveKinds is PC-19's own acceptance criterion, made concrete and
// then some: a single hand-verified scorecard pair producing all five DeltaKind
// values, each independently checked.
func TestComputeDelta_AllFiveKinds(t *testing.T) {
	old := map[string]string{
		"rds-encryption":   "unsatisfied",    // improves
		"waf-association":  "satisfied",      // regresses
		"cache-encryption": "satisfied",      // unchanged
		"iam-policy-scope": "not_assessable", // was unknown, still is — not_assessable
		"dlq-configured":   "satisfied",      // removed in new version (resource gone)
	}
	newV := map[string]string{
		"rds-encryption":   "satisfied",      // was unsatisfied — IMPROVEMENT
		"waf-association":  "unsatisfied",    // was satisfied — REGRESSION
		"cache-encryption": "satisfied",      // unchanged — UNCHANGED
		"iam-policy-scope": "not_assessable", // still unrankable — NOT_ASSESSABLE
		// dlq-configured absent — removed
		"new-security-group": "unsatisfied", // never existed before — NEW_RISK
	}

	entries := ComputeDelta(old, newV)
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want 6 (5 old/new IDs unioned minus overlap + 1 new)", len(entries))
	}

	if got := findEntry(t, entries, "rds-encryption").Kind; got != DeltaImprovement {
		t.Errorf("rds-encryption: Kind = %q, want improvement", got)
	}
	if got := findEntry(t, entries, "waf-association").Kind; got != DeltaRegression {
		t.Errorf("waf-association: Kind = %q, want regression", got)
	}
	if got := findEntry(t, entries, "cache-encryption").Kind; got != DeltaUnchanged {
		t.Errorf("cache-encryption: Kind = %q, want unchanged", got)
	}
	if got := findEntry(t, entries, "iam-policy-scope").Kind; got != DeltaNotAssessable {
		t.Errorf("iam-policy-scope: Kind = %q, want not_assessable", got)
	}
	if got := findEntry(t, entries, "dlq-configured").Kind; got != DeltaNotAssessable {
		t.Errorf("dlq-configured (removed): Kind = %q, want not_assessable — a removal must never claim a direction", got)
	}
	if got := findEntry(t, entries, "dlq-configured").Reason; got == "" {
		t.Error("dlq-configured: expected a non-empty reason for its not_assessable classification")
	}
	if got := findEntry(t, entries, "new-security-group").Kind; got != DeltaNewRisk {
		t.Errorf("new-security-group: Kind = %q, want new_risk", got)
	}
}

// TestComputeDelta_UnknownResolvesToSatisfied_IsResolvedRiskNotImprovement is PC-83's
// own regression test: this is the exact shape that was misclassified before the fix
// (BuildScorecard used to drop the finding from the map entirely when not_assessable,
// making ComputeDelta see it as brand-new rather than "present in both, now known").
func TestComputeDelta_UnknownResolvesToSatisfied_IsResolvedRiskNotImprovement(t *testing.T) {
	entries := ComputeDelta(
		map[string]string{"rds-rpo-feasibility": "not_assessable"},
		map[string]string{"rds-rpo-feasibility": "satisfied"},
	)
	entry := findEntry(t, entries, "rds-rpo-feasibility")
	if entry.Kind != DeltaResolvedRisk {
		t.Errorf("Kind = %q, want resolved_risk — an unknown resolving into a known-good state is not the same claim as a known state improving", entry.Kind)
	}
	if entry.OldStatus != "not_assessable" || entry.NewStatus != "satisfied" {
		t.Errorf("OldStatus/NewStatus = %q/%q, want not_assessable/satisfied", entry.OldStatus, entry.NewStatus)
	}
}

// TestComputeDelta_UnknownResolvesToUnsatisfied_IsNewRisk is the symmetric bad-news
// case: an unknown resolving into a confirmed problem.
func TestComputeDelta_UnknownResolvesToUnsatisfied_IsNewRisk(t *testing.T) {
	entries := ComputeDelta(
		map[string]string{"x": "not_assessable"},
		map[string]string{"x": "unsatisfied"},
	)
	if got := findEntry(t, entries, "x").Kind; got != DeltaNewRisk {
		t.Errorf("Kind = %q, want new_risk — an unknown resolving into a confirmed problem is a real new risk", got)
	}
}

// TestComputeDelta_KnownBecomesUnknown_IsNotAssessableWithReason is the reverse
// direction: information was lost between versions. Must never claim a direction, for
// the same reason a removed finding never does.
func TestComputeDelta_KnownBecomesUnknown_IsNotAssessableWithReason(t *testing.T) {
	entries := ComputeDelta(
		map[string]string{"x": "satisfied"},
		map[string]string{"x": "not_assessable"},
	)
	entry := findEntry(t, entries, "x")
	if entry.Kind != DeltaNotAssessable {
		t.Errorf("Kind = %q, want not_assessable — losing the ability to assess must never be reported as an improvement, regression, or unchanged", entry.Kind)
	}
	if entry.Reason == "" {
		t.Error("expected a non-empty reason explaining the information loss")
	}
}

func TestComputeDelta_NewFindingSatisfied_IsImprovementNotNewRisk(t *testing.T) {
	entries := ComputeDelta(map[string]string{}, map[string]string{"x": "satisfied"})
	if got := findEntry(t, entries, "x").Kind; got != DeltaImprovement {
		t.Errorf("a brand-new SATISFIED finding: Kind = %q, want improvement (a new good finding is a real improvement, not a risk)", got)
	}
}

func TestComputeDelta_NewFindingUnrankable_IsNotAssessable(t *testing.T) {
	entries := ComputeDelta(map[string]string{}, map[string]string{"x": "applicable"})
	if got := findEntry(t, entries, "x").Kind; got != DeltaNotAssessable {
		t.Errorf("a brand-new unrankable finding: Kind = %q, want not_assessable", got)
	}
}

func TestComputeDelta_EmptyBothSides_NoEntries(t *testing.T) {
	entries := ComputeDelta(map[string]string{}, map[string]string{})
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

// TestComputeDelta_Deterministic proves the same input always produces entries in the
// same order — I1's determinism requirement, applied here.
func TestComputeDelta_Deterministic(t *testing.T) {
	old := map[string]string{"a": "satisfied", "b": "unsatisfied", "c": "partial"}
	newV := map[string]string{"a": "unsatisfied", "b": "satisfied", "d": "unsatisfied"}

	first := ComputeDelta(old, newV)
	for i := 0; i < 10; i++ {
		got := ComputeDelta(old, newV)
		if len(got) != len(first) {
			t.Fatalf("run %d: length changed: %d vs %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("run %d: entry %d differs: %+v vs %+v", i, j, got[j], first[j])
			}
		}
	}
}
