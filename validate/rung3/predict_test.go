package rung3

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const (
	bundle   = "terraform"
	workload = "workload.yaml"
)

func TestPredict_MirrorsTerraform(t *testing.T) {
	if err := MirrorsTerraform("terraform/main.tf"); err != nil {
		t.Fatal(err)
	}
}

// The committed predictions.json must be exactly what the engine says today. It is committed
// BEFORE the experiment runs; if the engine or bundle later changes, this fails and the file must be
// regenerated deliberately (UPDATE_PREDICTIONS=1), not silently.
func TestPredict_MatchesCommittedPredictions(t *testing.T) {
	p, err := Predict(bundle, workload)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.MarshalIndent(p, "", "  ")
	got = append(got, '\n')
	if os.Getenv("UPDATE_PREDICTIONS") == "1" {
		if err := os.WriteFile("predictions.json", got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("predictions.json")
	if err != nil {
		t.Fatalf("predictions.json missing: %v (UPDATE_PREDICTIONS=1 to write it)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("predictions.json is stale; regenerate deliberately with UPDATE_PREDICTIONS=1\n--- got\n%s", got)
	}
}

func TestPredict_ClaimsAreWhatTheEngineSaid(t *testing.T) {
	p, err := Predict(bundle, workload)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]Statement{}
	for _, s := range p.Statements {
		ids[s.ID] = s
	}
	if !strings.Contains(ids["P1"].Claim, "web-b still flows") {
		t.Errorf("P1 should be the engine's surviving-path claim, got %q", ids["P1"].Claim)
	}
	for _, s := range p.Statements {
		if s.Provenance.Kind != "derived" {
			t.Errorf("%s: a prediction is derived, got %q", s.ID, s.Provenance.Kind)
		}
	}
	if len(p.EngineSilentOn) == 0 {
		t.Error("what the engine does not predict must be recorded, not omitted")
	}
}
