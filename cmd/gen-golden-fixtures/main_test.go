package main

// This file encodes the hand-verification of golden/fixtures/*.findings.json as a
// permanent, re-runnable test — the same discipline every other fixture-generating
// command in this project follows (contracts/, golden/aws's own IR fixtures): eyeball
// the first correct run once, then never trust it silently again on every subsequent
// regeneration.

import (
	"encoding/json"
	"os"
	"testing"

	"preflight/core"
)

func loadFindings(t *testing.T, path string) []core.Finding {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v — run `go run ./cmd/gen-golden-fixtures` first", path, err)
	}
	var findings []core.Finding
	if err := json.Unmarshal(data, &findings); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return findings
}

func findingByID(t *testing.T, findings []core.Finding, id string) core.Finding {
	t.Helper()
	for _, f := range findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("finding %q not found", id)
	return core.Finding{}
}

// TestFixtureFindings_AllValidateAgainstFrozenSchema is the baseline: every generated
// finding, in both bundles, must pass the same frozen-schema validation PC-7/17/18
// established — a fixture that doesn't validate against its own contract is worse
// than no fixture at all.
func TestFixtureFindings_AllValidateAgainstFrozenSchema(t *testing.T) {
	for _, path := range []string{"../../golden/fixtures/aws.findings.json", "../../golden/fixtures/aws-broken.findings.json"} {
		for _, f := range loadFindings(t, path) {
			if err := f.Validate(); err != nil {
				t.Errorf("%s: finding %q failed schema validation: %v", path, f.ID, err)
			}
		}
	}
}

// TestFixtureFindings_ZoneKillDataA_IsNotAssessable is THE case this whole line of
// work was about: the data tier's zone-kill finding must carry not_assessable with a
// real reason in BOTH bundles — not a bare empty/zero result that could be misread as
// "confirmed safe." See core/finding.go's DeriveImpact doc comment and golden/fixtures/
// README.md for why this is currently correct (unmapped subnet-group substrate) rather
// than a bug.
func TestFixtureFindings_ZoneKillDataA_IsNotAssessable(t *testing.T) {
	for _, path := range []string{"../../golden/fixtures/aws.findings.json", "../../golden/fixtures/aws-broken.findings.json"} {
		f := findingByID(t, loadFindings(t, path), "finding.zone-kill.data-a")
		if f.Outcome.State != core.AssessmentStateNotAssessable {
			t.Fatalf("%s: finding.zone-kill.data-a Outcome.State = %q, want not_assessable", path, f.Outcome.State)
		}
		if f.Outcome.Reason == "" {
			t.Fatalf("%s: finding.zone-kill.data-a has no stated reason for its not_assessable outcome (I4)", path)
		}
		if f.Dimensions.Impact.State != core.AssessmentStateNotAssessable {
			t.Errorf("%s: finding.zone-kill.data-a's Impact dimension = %q, want not_assessable", path, f.Dimensions.Impact.State)
		}
	}
}

// TestFixtureFindings_ZoneKillPublicA_IsAssessed is the positive control: a zone-kill
// with a real, known blast radius must NOT be not_assessable in either bundle — proves
// the not_assessable case above is a genuine "don't know", not a default every finding
// falls into.
func TestFixtureFindings_ZoneKillPublicA_IsAssessed(t *testing.T) {
	for _, path := range []string{"../../golden/fixtures/aws.findings.json", "../../golden/fixtures/aws-broken.findings.json"} {
		f := findingByID(t, loadFindings(t, path), "finding.zone-kill.public-a")
		if f.Outcome.State != core.AssessmentStateAssessed {
			t.Fatalf("%s: finding.zone-kill.public-a Outcome.State = %q, want assessed", path, f.Outcome.State)
		}
	}
}

// TestFixtureFindings_RDSEncryption_DiffersByBundle hand-verifies the one thing that
// MUST differ between the two bundles' fixtures: golden/aws-broken's defect 2.
func TestFixtureFindings_RDSEncryption_DiffersByBundle(t *testing.T) {
	clean := findingByID(t, loadFindings(t, "../../golden/fixtures/aws.findings.json"),
		"finding.compliance.rds-storage-encryption.aws_db_instance.payments")
	broken := findingByID(t, loadFindings(t, "../../golden/fixtures/aws-broken.findings.json"),
		"finding.compliance.rds-storage-encryption.aws_db_instance.payments")

	if clean.Outcome.Value != "satisfied" {
		t.Errorf("clean bundle: Outcome.Value = %v, want \"satisfied\"", clean.Outcome.Value)
	}
	if broken.Outcome.Value != "unsatisfied" {
		t.Errorf("broken bundle: Outcome.Value = %v, want \"unsatisfied\" (defect 2)", broken.Outcome.Value)
	}
}

// Regeneration determinism (does `go run ./cmd/gen-golden-fixtures` twice produce
// byte-identical output?) is checked manually, the same way contracts/'s own
// determinism is — see golden/fixtures/README.md's "Regenerating" section — not as a
// Go test here: a test that shells out to build and run this same binary to check its
// own output would be circular in a way that adds little over just running the
// command twice by hand, which is what the README already documents doing before
// committing a regeneration.
