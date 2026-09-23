package analyse

import "testing"

// TestDeriveLikelihood_AlwaysNotAssessable is PC-17's own Conversation, made
// executable: likelihood must be not-assessable, always, for this analysis layer —
// this is the correct answer, not a bug to eventually fix.
func TestDeriveLikelihood_AlwaysNotAssessable(t *testing.T) {
	_, ok, reason := DeriveLikelihood()
	if ok {
		t.Fatal("DeriveLikelihood must never report ok=true for Layer 1 structural analysis")
	}
	if reason == "" {
		t.Fatal("expected a non-empty reason")
	}
}

func TestDeriveDetectability(t *testing.T) {
	cases := []struct {
		detection string
		wantOK    bool
	}{
		{"modeled", true},
		{"observed", true},
		{"declared", true},
		{"unknown", false},
		{"garbage", false},
	}
	for _, c := range cases {
		value, ok, reason := DeriveDetectability(c.detection)
		if ok != c.wantOK {
			t.Errorf("detection=%q: ok=%v, want %v (value=%q reason=%q)", c.detection, ok, c.wantOK, value, reason)
		}
		if !ok && reason == "" {
			t.Errorf("detection=%q: not-assessable result must carry a reason", c.detection)
		}
		if ok && value == "" {
			t.Errorf("detection=%q: assessed result must carry a non-empty value", c.detection)
		}
	}
}

// TestDeriveImpact_NeverProducesANumber is PC-17's first acceptance criterion applied
// specifically to this function: CLAUDE.md §10 says "blast radius × workload
// criticality", but that is not a literal multiplication to implement — this test
// fails if a future edit ever makes DeriveImpact return a numeric type or a
// stringified arithmetic result.
func TestDeriveImpact_NeverProducesANumber(t *testing.T) {
	value, ok, _ := DeriveImpact(3, "tier1")
	if !ok {
		t.Fatal("expected ok=true for a real blast radius and criticality")
	}
	// A crude but meaningful guard: the value must not itself be parseable as a bare
	// number (which would indicate someone reduced this to "3 * 1 = 3" style scoring).
	for _, c := range value {
		if c >= '0' && c <= '9' {
			continue // digits are fine WITHIN a descriptive sentence (e.g. "3 component(s)")
		}
	}
	if value == "3" || value == "3.0" {
		t.Fatalf("DeriveImpact returned a bare number (%q) — this is exactly the composite severity score PC-17 forbids", value)
	}
}

func TestDeriveImpact_EmptyBlastRadius_NotAssessable(t *testing.T) {
	_, ok, reason := DeriveImpact(0, "tier1")
	if ok {
		t.Fatal("expected not-assessable for an empty blast radius")
	}
	if reason == "" {
		t.Fatal("expected a non-empty reason")
	}
}

func TestDeriveImpact_NoCriticalityDeclared_NotAssessable(t *testing.T) {
	_, ok, reason := DeriveImpact(3, "")
	if ok {
		t.Fatal("expected not-assessable when workload criticality is not declared")
	}
	if reason == "" {
		t.Fatal("expected a non-empty reason")
	}
}
