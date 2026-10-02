// Package rung3 is PC-25: the Rung 3 experiment runner. It applies the bundle in validate/rung3,
// injects one real fault with AWS FIS, captures what the load balancer and the clients actually
// saw, and destroys everything, in one run.
//
// It lives under cmd/runnerd/internal/ so that Go itself keeps every other process from importing
// it: P3 is the only process allowed to hold cloud credentials (ADR-003). The runner never reads a
// key. It runs `terraform` and `aws`, which resolve credentials from the environment they inherit.
//
// This is also the only place in the codebase that can produce a Result typed observed with the
// resilience tier (I5). core.Provenance already rejects that tier for any rung but 3, and
// TestObservedIsOnlyProducedHere scans the module so a second producer cannot appear unnoticed.
package rung3

import (
	"fmt"
	"time"

	"preflight/core"
)

// Source names the producer in every provenance this package issues.
const Source = "runnerd:rung3"

// observedProvenance is the ONLY constructor of an observed provenance in non-test code. The tier
// is resilience (failover timing is exactly what only a real cloud can show) and the rung is 3.
func observedProvenance(what string) core.Provenance {
	p := core.NewProvenance(core.KindObserved, Source+":"+what).
		WithObserved(core.EvidenceTierResilience, core.Rung3RealCloud)
	if err := p.Validate(); err != nil {
		panic(fmt.Sprintf("rung3: invalid observed provenance: %v", err)) // a programming error, never data
	}
	return p
}

// Probe is one client request to the load balancer.
type Probe struct {
	AtMs       int64  `json:"at_ms"` // since the fault was started (negative = baseline)
	DurationMs int64  `json:"duration_ms"`
	Status     int    `json:"status"` // 0 when no HTTP response arrived
	Body       string `json:"body,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Failed reports whether the client did not get a 200 from a web instance.
func (p Probe) Failed() bool { return p.Status != 200 }

// HealthSample is the target group's own view of each target at one moment.
type HealthSample struct {
	AtMs    int64             `json:"at_ms"`
	Targets map[string]Target `json:"targets"` // keyed by web-a / web-b
}

// Target is one registered target's reported health.
type Target struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

// Observed is what was measured. Every number comes from the run; none is derived from the model.
type Observed struct {
	ExperimentState        string   `json:"experiment_state"`
	BaselineProbes         int      `json:"baseline_probes"`
	BaselineFailures       int      `json:"baseline_failures"`
	BaselineServedBy       []string `json:"baseline_served_by"`
	ProbesAfterFault       int      `json:"probes_after_fault"`
	FailuresAfterFault     int      `json:"failures_after_fault"`
	FirstFailureSeconds    *float64 `json:"first_failure_seconds,omitempty"`
	LastFailureSeconds     *float64 `json:"last_failure_seconds,omitempty"`
	FailureWindowSeconds   *float64 `json:"failure_window_seconds,omitempty"`
	FailureShareInWindow   *float64 `json:"failure_share_in_window,omitempty"`
	ServedByAfterLastFail  []string `json:"served_by_after_last_failure"`
	WebAFirstNotHealthySec *float64 `json:"web_a_first_not_healthy_seconds,omitempty"`
	WebAFinalState         string   `json:"web_a_final_state"`
	WebAFinalReason        string   `json:"web_a_final_reason,omitempty"`
}

// Comparison is one prediction checked against the run.
type Comparison struct {
	ID       string `json:"id"`
	Claim    string `json:"claim"`
	Verdict  string `json:"verdict"` // confirmed | refuted | not_observed
	Evidence string `json:"evidence"`
}

// Cleanup records what destroy left behind. Clean is only true when both checks agree.
type Cleanup struct {
	Destroyed      bool     `json:"destroyed"`
	StateEmpty     bool     `json:"state_empty"`
	TaggedLeftover []string `json:"tagged_leftover,omitempty"`
	Clean          bool     `json:"clean"`
	Note           string   `json:"note,omitempty"`
}

// Result is one complete Rung 3 run.
type Result struct {
	Experiment string          `json:"experiment"`
	RunID      string          `json:"run_id"`
	Account    string          `json:"account"`
	Region     string          `json:"region"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at"`
	Provenance core.Provenance `json:"provenance"`
	Observed   Observed        `json:"observed"`
	Comparison []Comparison    `json:"comparison"`
	Cleanup    Cleanup         `json:"cleanup"`
	Probes     []Probe         `json:"probes"`
	Health     []HealthSample  `json:"health"`
}
