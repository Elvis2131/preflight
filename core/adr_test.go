package core_test

// PC-20's own acceptance criteria, hand-verified: (1) Waiver schema captures who/what
// finding/when/why/against which version/optional expiry — checked structurally below;
// (2) a waived finding displays as accepted in the scorecard, not hidden entirely —
// checked behaviorally via core.ApplyWaivers; (3) ADR objects link to failure modes and
// controls by ID — checked structurally below. See core/adr.go's own doc comments for
// the full design reasoning (especially Waiver.AgainstVersion being a >= floor, not an
// exact-match gate).

import (
	"reflect"
	"testing"
	"time"

	"preflight/core"
)

func TestWaiverSchemaCapturesRequiredFields(t *testing.T) {
	typ := reflect.TypeOf(core.Waiver{})
	for _, want := range []string{"AcceptedBy", "FindingID", "AcceptedAt", "Reason", "AgainstVersion", "ExpiresAt"} {
		if _, ok := typ.FieldByName(want); !ok {
			t.Errorf("core.Waiver has no %s field", want)
		}
	}
}

func TestADRLinksFailureModesAndControlsByID(t *testing.T) {
	adr := core.ADR{
		ID:                    "adr.1",
		Decision:              "Accept single-AZ RDS in staging",
		Reason:                "Cost outweighs risk for a non-production environment",
		ControlsSatisfied:     []string{"control.rds-storage-encryption"},
		FailureModesAddressed: []string{"finding.zone-kill.data-a"},
		Provenance:            core.NewProvenance(core.KindStated, "test"),
	}
	if err := adr.Validate(); err != nil {
		t.Fatalf("valid ADR failed schema validation: %v", err)
	}
	// "linked by ID" (PRD §4) means these are plain string IDs, not embedded copies of
	// the objects they reference — checked structurally so a future change back to
	// embedding would fail loudly here, not just get discovered by inspection.
	typ := reflect.TypeOf(core.ADR{})
	for _, name := range []string{"ControlsSatisfied", "FailureModesAddressed"} {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("core.ADR has no %s field", name)
		}
		if f.Type.Kind() != reflect.Slice || f.Type.Elem().Kind() != reflect.String {
			t.Errorf("core.ADR.%s = %s, want []string (linked by ID, not an embedded object)", name, f.Type)
		}
	}
}

func mustWaiver(t *testing.T, findingID string, againstVersion int, expiresAt *string) core.Waiver {
	t.Helper()
	w := core.Waiver{
		AcceptedBy:     "alice@example.com",
		FindingID:      findingID,
		AcceptedAt:     "2026-01-01T00:00:00Z",
		Reason:         "accepted for this quarter's cost/risk trade-off",
		AgainstVersion: againstVersion,
		ExpiresAt:      expiresAt,
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("test waiver failed its own schema validation: %v", err)
	}
	return w
}

func TestWaiverApplies_VersionFloorNotExactMatch(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	w := mustWaiver(t, "finding.x", 3, nil)

	cases := []struct {
		version int
		want    bool
	}{
		{2, false}, // waiver accepted against version 3 says nothing about an earlier version
		{3, true},  // exact match: applies
		{4, true},  // later version: still applies — this is the whole point of the object
		{99, true},
	}
	for _, c := range cases {
		if got := w.Applies(c.version, now); got != c.want {
			t.Errorf("Applies(version=%d) = %v, want %v", c.version, got, c.want)
		}
	}
}

func TestWaiverApplies_Expiry(t *testing.T) {
	future := "2027-01-01T00:00:00Z"
	past := "2025-01-01T00:00:00Z"
	malformed := "not-a-date"
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if !mustWaiver(t, "finding.x", 1, nil).Applies(5, now) {
		t.Error("no ExpiresAt should mean never expires")
	}
	if !mustWaiver(t, "finding.x", 1, &future).Applies(5, now) {
		t.Error("a future ExpiresAt should still apply")
	}
	if mustWaiver(t, "finding.x", 1, &past).Applies(5, now) {
		t.Error("a past ExpiresAt should no longer apply")
	}
	if mustWaiver(t, "finding.x", 1, &malformed).Applies(5, now) {
		t.Error("an unparsable ExpiresAt should be treated as expired, not as valid")
	}
}

func TestApplyWaivers_MarksAcceptedWithoutHidingEntries(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "test")
	sc := core.Scorecard{
		VersionNumber: 5,
		Entries: []core.ScorecardEntry{
			{FindingID: "finding.waived", Status: "unsatisfied", Provenance: prov},
			{FindingID: "finding.untouched", Status: "satisfied", Provenance: prov},
			{FindingID: "finding.expired-waiver", Status: "unsatisfied", Provenance: prov},
			{FindingID: "finding.future-version-waiver", Status: "unsatisfied", Provenance: prov},
		},
	}
	past := "2025-01-01T00:00:00Z"
	waivers := []core.Waiver{
		mustWaiver(t, "finding.waived", 2, nil),
		mustWaiver(t, "finding.expired-waiver", 1, &past),
		mustWaiver(t, "finding.future-version-waiver", 10, nil), // accepted against a version not reached yet
	}

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := core.ApplyWaivers(sc, waivers, now)

	if len(got.Entries) != len(sc.Entries) {
		t.Fatalf("ApplyWaivers changed entry count: got %d, want %d — waived findings must display, not be hidden", len(got.Entries), len(sc.Entries))
	}

	want := map[string]string{
		"finding.waived":                "accepted",
		"finding.untouched":             "satisfied",
		"finding.expired-waiver":        "unsatisfied",
		"finding.future-version-waiver": "unsatisfied",
	}
	statusMap := got.StatusMap()
	for id, wantStatus := range want {
		if got := statusMap[id]; got != wantStatus {
			t.Errorf("%s: Status = %q, want %q", id, got, wantStatus)
		}
	}

	if err := got.Validate(); err != nil {
		t.Fatalf("scorecard with waivers applied failed schema validation: %v", err)
	}
}
