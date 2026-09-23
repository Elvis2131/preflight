package analyse

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestStorageEncryptionCheck_Encrypted_Satisfied(t *testing.T) {
	status, rationale := StorageEncryptionCheck(boolPtr(true))
	if status != "satisfied" {
		t.Errorf("status = %q, want satisfied", status)
	}
	if rationale == "" {
		t.Error("expected a non-empty rationale")
	}
}

func TestStorageEncryptionCheck_Unencrypted_Unsatisfied(t *testing.T) {
	status, rationale := StorageEncryptionCheck(boolPtr(false))
	if status != "unsatisfied" {
		t.Errorf("status = %q, want unsatisfied", status)
	}
	if rationale == "" {
		t.Error("expected a non-empty rationale")
	}
}

// TestStorageEncryptionCheck_Absent_NotAssessable is the "no bare pass/fail" test
// applied to this specific control: an absent attribute must never be treated as
// "false" (which would fabricate a violation from missing data) or silently skipped —
// it must be its own, distinct not_assessable outcome.
func TestStorageEncryptionCheck_Absent_NotAssessable(t *testing.T) {
	status, rationale := StorageEncryptionCheck(nil)
	if status != "not_assessable" {
		t.Errorf("status = %q, want not_assessable — an absent attribute must never be silently read as false", status)
	}
	if rationale == "" {
		t.Error("expected a non-empty rationale")
	}
}

// TestStorageEncryptionCheck_NeverReturnsABareBool is PC-18's third acceptance
// criterion checked at the function-signature level: the return type itself is a
// string drawn from the 5-value ComplianceStatus vocabulary, not a bool — a future
// edit that changed this signature to `(bool, string)` would fail to compile against
// core/compliance.go's ComplianceResult usage, but this test also documents the
// intent directly for a reader of this file alone.
func TestStorageEncryptionCheck_NeverReturnsABareBool(t *testing.T) {
	for _, input := range []*bool{nil, boolPtr(true), boolPtr(false)} {
		status, _ := StorageEncryptionCheck(input)
		switch status {
		case "applicable", "satisfied", "partial", "unsatisfied", "not_assessable":
			// valid
		default:
			t.Errorf("input=%v: status %q is not one of the 5 defined ComplianceStatus values", input, status)
		}
	}
}
