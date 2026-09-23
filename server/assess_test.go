package server

import (
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// TestAssess_FirstVersion_NoDeltaYet is the baseline: a brand-new session's first
// /assess call produces Version 1 with no AssuranceDelta (nothing to diff against).
func TestAssess_FirstVersion_NoDeltaYet(t *testing.T) {
	store := testStore(t)
	resp, err := Assess(store, AssessRequest{
		SessionID:    "sess-1",
		BundleDir:    "../golden/aws",
		WorkloadPath: "../golden/workload.yaml",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.VersionNumber != 1 {
		t.Errorf("VersionNumber = %d, want 1", resp.VersionNumber)
	}
	if resp.AssuranceDelta != nil {
		t.Errorf("AssuranceDelta = %v, want nil for a session's first version", resp.AssuranceDelta)
	}
	if len(resp.Findings) == 0 {
		t.Error("expected real findings")
	}
}

// TestAssess_SecondVersion_RealDelta re-runs the SAME session against golden/aws-broken
// then golden/aws — proving Version N semantics and a real, non-nil AssuranceDelta
// actually persist and compute correctly through the store, not just in one call.
func TestAssess_SecondVersion_RealDelta(t *testing.T) {
	store := testStore(t)

	v1, err := Assess(store, AssessRequest{SessionID: "sess-2", BundleDir: "../golden/aws-broken", WorkloadPath: "../golden/workload.yaml"})
	if err != nil {
		t.Fatalf("Assess (v1): %v", err)
	}
	if v1.VersionNumber != 1 {
		t.Fatalf("v1.VersionNumber = %d, want 1", v1.VersionNumber)
	}

	v2, err := Assess(store, AssessRequest{SessionID: "sess-2", BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"})
	if err != nil {
		t.Fatalf("Assess (v2): %v", err)
	}
	if v2.VersionNumber != 2 {
		t.Fatalf("v2.VersionNumber = %d, want 2", v2.VersionNumber)
	}
	if len(v2.AssuranceDelta) == 0 {
		t.Fatal("expected a real, non-empty AssuranceDelta on the second call")
	}

	foundImprovement := false
	for _, e := range v2.AssuranceDelta {
		if e.FindingID == "finding.compliance.rds-storage-encryption.aws_db_instance.payments" {
			if e.Kind != "improvement" {
				t.Errorf("RDS finding delta Kind = %q, want improvement", e.Kind)
			}
			foundImprovement = true
		}
	}
	if !foundImprovement {
		t.Fatal("expected the RDS encryption finding in the delta")
	}
}

// TestAssess_Latency_UnderFiveSeconds is PC-21's first acceptance criterion, measured,
// against the real golden fixture — not asserted, timed.
func TestAssess_Latency_UnderFiveSeconds(t *testing.T) {
	store := testStore(t)
	start := time.Now()
	resp, err := Assess(store, AssessRequest{SessionID: "sess-latency", BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Assess took %v, want under 5s", elapsed)
	}
	if resp.ComputeDurationMS <= 0 {
		t.Error("ComputeDurationMS should be a real, positive measured value")
	}
	if resp.ComputeDurationMS > 5000 {
		t.Errorf("self-reported ComputeDurationMS = %d, want under 5000", resp.ComputeDurationMS)
	}
}

// TestAssess_AlwaysDegraded_NoLLMClientExists documents, and tests, the honest current
// state: reason/ has no client at all, so Degraded is unconditionally true today. This
// is not "the LLM timed out" being simulated — there is no LLM call in this path to
// time out — and the test says so rather than implying a real degradation scenario was
// exercised.
func TestAssess_AlwaysDegraded_NoLLMClientExists(t *testing.T) {
	store := testStore(t)
	resp, err := Assess(store, AssessRequest{SessionID: "sess-degraded", BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if !resp.Degraded {
		t.Error("Degraded = false, want true — reason/ has no LLM client wired up yet")
	}
	if resp.DegradedReason == "" {
		t.Error("expected a non-empty DegradedReason")
	}
}

// TestAssess_GraphIsRealSVGNeverOmitted was PC-21's own named scope exclusion (a
// stub); PC-81 landed real rendering (core.RenderDOT + render.SVG), so this now
// asserts the field is always present with REAL SVG content — never nil, never an
// error, never silently absent — not merely a non-empty stub string.
func TestAssess_GraphIsRealSVGNeverOmitted(t *testing.T) {
	store := testStore(t)
	resp, err := Assess(store, AssessRequest{SessionID: "sess-graph", BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if resp.Graph == nil {
		t.Fatal("Graph must never be nil — always a real value, never an omitted field")
	}
	if !strings.Contains(*resp.Graph, "<svg") {
		t.Errorf("Graph = %.100s..., want real SVG content (an <svg> tag)", *resp.Graph)
	}
}

func TestAssess_MissingSessionID_Errors(t *testing.T) {
	store := testStore(t)
	_, err := Assess(store, AssessRequest{BundleDir: "../golden/aws", WorkloadPath: "../golden/workload.yaml"})
	if err == nil {
		t.Fatal("expected an error for a missing session_id")
	}
}
