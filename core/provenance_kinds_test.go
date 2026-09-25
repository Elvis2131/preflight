package core_test

import (
	"testing"

	"preflight/core"
)

// TestAssumedRequiresReason and TestLLMReasonedRequiresReason are PRD §4's explicit
// pairing rules: "assumed (a declared default the user can override)" and
// "llm_reasoned (LLM contextual judgment, always citing IR evidence)" both require a
// stated reason — an assumed or llm_reasoned value with no explanation is exactly the
// kind of untraceable assertion I2 exists to make unrepresentable.
func TestAssumedRequiresReason(t *testing.T) {
	p := core.NewProvenance(core.KindAssumed, "workload.yaml:regions")
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error: assumed provenance with no Reason")
	}
	p = p.WithReason("no region declared; defaulted to eu-west-1 per PRD §4 fallback policy")
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() after WithReason = %v, want nil", err)
	}
}

func TestLLMReasonedRequiresReason(t *testing.T) {
	p := core.NewProvenance(core.KindLLMReasoned, "reason/narrative:finding-042")
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error: llm_reasoned provenance with no Reason (must cite IR evidence)")
	}
}

// TestObservedRequiresEvidenceTierAndRung is I5's mechanism: an observed value must
// declare which evidence tier it can support and which validation-ladder rung produced
// it, so a Rung-2 (emulated) result can never later be read as resilience evidence.
func TestObservedRequiresEvidenceTierAndRung(t *testing.T) {
	p := core.NewProvenance(core.KindObserved, "validate/rung3:experiment-2026-09-20")
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error: observed provenance with no EvidenceTier/Rung")
	}
	p = p.WithObserved(core.EvidenceTierResilience, core.Rung3RealCloud)
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() after WithObserved = %v, want nil", err)
	}
}

// TestNonObservedRejectsEvidenceTier proves the pairing is enforced in both directions:
// a stated/derived/assumed value cannot smuggle in an evidence tier it never earned —
// this is the structural half of I5, independent of which rung actually ran.
func TestNonObservedRejectsEvidenceTier(t *testing.T) {
	p := core.NewProvenance(core.KindStated, "workload.yaml:availability.target")
	tier := core.EvidenceTierResilience
	rung := core.Rung3RealCloud
	p.EvidenceTier = &tier
	p.Rung = &rung
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error: non-observed provenance carrying an EvidenceTier/Rung")
	}
}

// TestObservedEvidenceTierCannotExceedItsRung is I5's OTHER half (PC-24): only Rung 3
// (a real ephemeral cloud experiment) may carry EvidenceTierPerformance/Resilience —
// Rung 1 (a local topology replica, PC-24's own Toxiproxy harness) and Rung 2 (LocalStack
// emulation) can only ever produce EvidenceTierFunctional. Before this test (and the
// validator registration it exercises), nothing anywhere actually enforced this —
// EvidenceTierPerformance/EvidenceTierResilience's own doc comments stated the rule in
// prose only.
func TestObservedEvidenceTierCannotExceedItsRung(t *testing.T) {
	cases := []struct {
		name    string
		tier    core.EvidenceTier
		rung    core.Rung
		wantErr bool
	}{
		{"functional/rung1: allowed", core.EvidenceTierFunctional, core.Rung1TopologyReplica, false},
		{"functional/rung2: allowed", core.EvidenceTierFunctional, core.Rung2Emulated, false},
		{"functional/rung3: allowed", core.EvidenceTierFunctional, core.Rung3RealCloud, false},
		{"performance/rung1: rejected — a topology replica cannot support a throughput/latency claim", core.EvidenceTierPerformance, core.Rung1TopologyReplica, true},
		{"performance/rung2: rejected — LocalStack emulation cannot support a throughput/latency claim", core.EvidenceTierPerformance, core.Rung2Emulated, true},
		{"performance/rung3: allowed", core.EvidenceTierPerformance, core.Rung3RealCloud, false},
		{"resilience/rung1: rejected — a topology replica cannot support an RTO/RPO/failover-time claim", core.EvidenceTierResilience, core.Rung1TopologyReplica, true},
		{"resilience/rung2: rejected — LocalStack emulation cannot support an RTO/RPO/failover-time claim", core.EvidenceTierResilience, core.Rung2Emulated, true},
		{"resilience/rung3: allowed", core.EvidenceTierResilience, core.Rung3RealCloud, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := core.NewProvenance(core.KindObserved, "validate/rungN:test").WithObserved(c.tier, c.rung)
			err := p.Validate()
			if c.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want an error (tier %s cannot be carried by rung %v)", c.tier, c.rung)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestAssessmentEnvelopeRoundTrip(t *testing.T) {
	prov := core.NewProvenance(core.KindDerived, "core/analyse:spof-detector")

	assessed := core.Assessed(true, prov).ToEnvelope()
	if err := assessed.Validate(); err != nil {
		t.Fatalf("assessed envelope Validate() = %v, want nil", err)
	}

	unresolved := core.NotAssessable[bool]("dynamic block not in v1 parser subset", prov).ToEnvelope()
	if err := unresolved.Validate(); err != nil {
		t.Fatalf("not_assessable envelope Validate() = %v, want nil", err)
	}

	// I4's structural guarantee on the wire: an envelope claiming "assessed" but
	// carrying a Reason (or claiming not_assessable while carrying a Value) must fail
	// validation — the schema must reject this shape even though nothing in Go stops
	// you from hand-building a malformed envelope literal.
	malformed := core.AssessmentEnvelope{
		State:      core.AssessmentStateAssessed,
		Reason:     "should not be here",
		Provenance: prov,
	}
	if err := malformed.Validate(); err == nil {
		t.Fatal("expected validation error: assessed envelope carrying a Reason")
	}
}
