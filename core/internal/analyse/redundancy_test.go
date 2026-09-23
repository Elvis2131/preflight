package analyse

import "testing"

func TestNATGatewayRedundancyCheck_AllCovered_Satisfied(t *testing.T) {
	status, rationale := NATGatewayRedundancyCheck(map[string]int{
		"public_a": 1, "public_b": 1, "public_c": 1,
	})
	if status != "satisfied" {
		t.Errorf("status = %q, want satisfied", status)
	}
	if rationale == "" {
		t.Error("expected a non-empty rationale")
	}
}

// TestNATGatewayRedundancyCheck_SharedSinglePoint_Unsatisfied is exactly golden/
// aws-broken's defect 1 shape: public_a has the one remaining gateway, public_b/c
// have none of their own.
func TestNATGatewayRedundancyCheck_SharedSinglePoint_Unsatisfied(t *testing.T) {
	status, rationale := NATGatewayRedundancyCheck(map[string]int{
		"public_a": 1, "public_b": 0, "public_c": 0,
	})
	if status != "unsatisfied" {
		t.Errorf("status = %q, want unsatisfied", status)
	}
	if rationale == "" {
		t.Error("expected a non-empty rationale naming the uncovered subnets")
	}
}

func TestNATGatewayRedundancyCheck_NoSubnetsGiven_NotAssessable(t *testing.T) {
	status, _ := NATGatewayRedundancyCheck(map[string]int{})
	if status != "not_assessable" {
		t.Errorf("status = %q, want not_assessable", status)
	}
}

func TestNATGatewayRedundancyCheck_NeverReturnsABareBool(t *testing.T) {
	for _, input := range []map[string]int{
		{}, {"a": 0}, {"a": 1}, {"a": 1, "b": 0},
	} {
		status, _ := NATGatewayRedundancyCheck(input)
		switch status {
		case "applicable", "satisfied", "partial", "unsatisfied", "not_assessable":
		default:
			t.Errorf("input=%v: status %q is not one of the 5 defined ComplianceStatus values", input, status)
		}
	}
}
