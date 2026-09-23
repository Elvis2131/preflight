package analyse

import "testing"

// This file exists specifically because PC-14's own Conversation names capacity
// discipline as "the rule most likely to get violated by accident during
// implementation" and asks for a dedicated test trying to break it — every test below
// is an attempt to make SurvivingCapacity produce a numeric answer from instance count
// alone, in some form. None should succeed.
//
// Tests the plain-tuple signature directly (this package has no core dependency); the
// core.Assessment[float64]-wrapped behavior is tested again, end to end, in
// core/capacity_test.go.

func TestSurvivingCapacity_DeclaredCapacity_ComputesFromDeclaration(t *testing.T) {
	declared := map[string]float64{"app_node_rps": 500}

	value, ok, _ := SurvivingCapacity(declared, "app_node_rps", 3)
	if !ok {
		t.Fatal("expected ok=true when capacity IS declared")
	}
	if value != 1500 {
		t.Errorf("value = %v, want 1500 (3 surviving instances x 500 declared per-instance rps)", value)
	}
}

// TestSurvivingCapacity_UndeclaredKey_NeverComputesFromInstanceCountAlone is the core
// break-it attempt: capacity key absent entirely. This must be not-ok regardless of
// how many instances survive — including a LARGE instance count, which is exactly the
// case where "just multiply" would look most tempting and most plausible if it slipped
// through.
func TestSurvivingCapacity_UndeclaredKey_NeverComputesFromInstanceCountAlone(t *testing.T) {
	declared := map[string]float64{"other_tier_rps": 999} // present, but NOT the key being asked about

	for _, instances := range []int{0, 1, 3, 1000} {
		_, ok, reason := SurvivingCapacity(declared, "app_node_rps", instances)
		if ok {
			t.Fatalf("instances=%d: got ok=true, want not-assessable — app_node_rps was never declared", instances)
		}
		if reason == "" {
			t.Fatalf("instances=%d: expected a non-empty not-assessable reason", instances)
		}
		if !containsAll(reason, "capacity_unknown", "app_node_rps") {
			t.Errorf("instances=%d: reason %q should name both the capacity_unknown state and the missing key", instances, reason)
		}
	}
}

// TestSurvivingCapacity_NilDeclaredMap_StillNotAssessable — an entirely absent
// declaration (nil map, not just a missing key in a populated one) must behave
// identically to the missing-key case, not panic and not silently treat nil as zero
// capacity (which would itself be a fabricated value, not an honest "unknown").
func TestSurvivingCapacity_NilDeclaredMap_StillNotAssessable(t *testing.T) {
	_, ok, _ := SurvivingCapacity(nil, "app_node_rps", 5)
	if ok {
		t.Fatal("expected not-assessable for a nil capacity map")
	}
}

// TestSurvivingCapacity_ZeroDeclaredValue_IsStillADeclaration — a DECLARED zero is a
// real, stated fact (the workload author explicitly said "zero"), semantically
// different from "not declared at all". This must be ok=true with value 0, not
// conflated with not-assessable — a capacity check that can't tell "declared zero"
// from "unknown" would be exactly as unreliable as one that infers from instance
// count, just failing in the opposite direction.
func TestSurvivingCapacity_ZeroDeclaredValue_IsStillADeclaration(t *testing.T) {
	declared := map[string]float64{"app_node_rps": 0}
	value, ok, _ := SurvivingCapacity(declared, "app_node_rps", 5)
	if !ok {
		t.Fatal("a declared zero must still be ok=true, not not-assessable — the value IS known, it's zero")
	}
	if value != 0 {
		t.Errorf("value = %v, want 0", value)
	}
}

// TestSurvivingCapacity_ZeroSurvivingInstances_WithRealDeclaration — the other edge:
// capacity IS declared, but nothing survived. This is a legitimate ok=true/value=0,
// not a reason to fall back to not-assessable — the answer is knowable and it's
// exactly zero.
func TestSurvivingCapacity_ZeroSurvivingInstances_WithRealDeclaration(t *testing.T) {
	declared := map[string]float64{"app_node_rps": 500}
	value, ok, _ := SurvivingCapacity(declared, "app_node_rps", 0)
	if !ok {
		t.Fatal("expected ok=true, value=0 when capacity is declared but zero instances survived")
	}
	if value != 0 {
		t.Errorf("value = %v, want 0", value)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !stringsContains(s, sub) {
			return false
		}
	}
	return true
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
