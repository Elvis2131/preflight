// PC-128: Layer 3 (PRD §5.5) — degraded-state latency and saturation from DECLARED
// service rates. Pure and deterministic (I1): closed-form queueing formulas, no
// randomness at all, so there is no seed to inject and none is needed — Seed on every
// result says exactly that rather than leaving the field's absence to be guessed at.
//
// What this is NOT, per the Card's own explicit exclusions: no SLA-derived failure
// rates, no Monte Carlo over provider SLAs, no number that is not traceable to a
// declared input (the Layer 2 rejection in CLAUDE.md §11 still holds). Every input
// used is listed next to every result.
package core

import (
	"fmt"
	"math"
	"sort"
)

// LatencySeedNote is stamped on every estimate: the model is closed-form, so there is
// no random seed (I1 allows an injected seed; this model simply never needs one).
const LatencySeedNote = "none: closed-form M/M/1 (M/G/1 where a service-time SCV is declared) formulas, no randomness anywhere in this model"

// latencyAssumptions are the modelling assumptions every estimate depends on. They
// are not declared by the architect and are not facts about the design — they are the
// price of using a closed form, so they are tagged assumed and shown with the result.
var latencyAssumptions = []string{
	"each component is one aggregate station: Poisson arrivals, service rate = the component's declared capacity (Workload.Capacity), and — unless Workload.ServiceTimeSCV declares otherwise for that component type — exponential service (M/M/1). A Rung 1 comparison on a real Postgres (docs/PREDICTED_VS_OBSERVED.md) found M/M/1 overestimates the mean latency of a near-deterministic service by 30-40% at high utilisation and is far too optimistic for a service with long stalls; declare the service-time SCV when it is known",
	"stations are independent in tandem (Jackson network), so a journey's mean latency is the sum of its components' mean sojourn times",
	"where a path position has more than one reached member, the worst (highest mean) member is used",
	"concurrency limits and request-size-dependent service time are not modelled",
}

// LatencyInput is one declared value the estimate was computed from — "the report must
// show the inputs next to the result".
type LatencyInput struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	// Source is always "stated": every input here comes from the workload declaration.
	Source string `json:"source"`
}

// ComponentLatency is one component's own queueing result. MeanMS/P95MS are nil when
// the component is saturated (offered load >= declared capacity): no steady state
// exists, so any finite number would be invented.
type ComponentLatency struct {
	NodeID      string   `json:"node_id"`
	OfferedRPS  float64  `json:"offered_rps"`
	CapacityRPS float64  `json:"capacity_rps"`
	Utilization float64  `json:"utilization"`
	Saturated   bool     `json:"saturated"`
	MeanMS      *float64 `json:"mean_ms,omitempty"`
	// P95MS is nil when saturated, and also whenever the component's service-time SCV is
	// DECLARED: the p95 = mean*ln(20) closed form only exists for exponential sojourn times,
	// and no honest closed form exists for a general service distribution.
	P95MS *float64 `json:"p95_ms,omitempty"`
	// ServiceTimeSCV is the squared coefficient of variation the mean was computed with;
	// SCVDeclared says whether the architect stated it (true) or the model assumed
	// exponential service (false, SCV = 1).
	ServiceTimeSCV float64 `json:"service_time_scv"`
	SCVDeclared    bool    `json:"scv_declared"`
}

// JourneyLatencyEstimate is the value of an assessed JourneyLatency.Result.
type JourneyLatencyEstimate struct {
	// Statement is the result framed as conditional on the declared inputs — never a
	// bare prediction.
	Statement string `json:"statement"`
	// MeanMS is nil when any component on the path is saturated.
	MeanMS      *float64           `json:"mean_ms,omitempty"`
	SaturatedAt string             `json:"saturated_at,omitempty"`
	Components  []ComponentLatency `json:"components"`
	Inputs      []LatencyInput     `json:"inputs"`
	Assumptions []string           `json:"assumptions"`
	Seed        string             `json:"seed"`
}

// JourneyLatency pairs a journey with its latency result: assessed (a
// JourneyLatencyEstimate) or not_assessable naming exactly what is missing or why the
// journey has no latency to estimate.
type JourneyLatency struct {
	JourneyID string             `json:"journey_id"`
	Result    AssessmentEnvelope `json:"result"`
}

// ComputeDegradedLatency estimates, for every declared journey, latency under the given
// fault (killed may be nil for the no-fault baseline — same convention as
// ComputeJourneyFlow/ComputeComponentLoad, whose results this is built on, so the
// offered load already reflects rerouting and killed components).
//
// A journey is assessable only if EvaluateJourneyLoadReadiness says every declared
// input is present (peak/steady rps, every path component resolvable, a declared
// capacity per component type) — its reason is passed through, naming the missing
// input; no service time or capacity is ever defaulted. A journey that does not flow
// under the fault has no latency to estimate: not_assessable, saying where it is
// blocked (the flow result, not this function, owns that verdict).
func ComputeDegradedLatency(ir *IR, workload Workload, killed map[string]bool, prov Provenance) []JourneyLatency {
	loads := map[string]ComponentLoad{}
	for _, l := range ComputeComponentLoad(ir, workload, killed) {
		loads[l.NodeID] = l
	}

	out := make([]JourneyLatency, 0, len(workload.Journeys))
	for _, j := range workload.Journeys {
		out = append(out, JourneyLatency{JourneyID: j.ID, Result: journeyLatency(ir, workload, j, killed, loads, prov)})
	}
	return out
}

func journeyLatency(ir *IR, workload Workload, j DeclaredJourney, killed map[string]bool, loads map[string]ComponentLoad, prov Provenance) AssessmentEnvelope {
	if ready := EvaluateJourneyLoadReadiness(ir, workload, j, prov); ready.State != AssessmentStateAssessed {
		return ready
	}
	flow := ComputeJourneyFlow(ir, j, killed)
	if !flow.Flows {
		return NotAssessable[any](fmt.Sprintf("journey %q does not flow under this fault (blocked at %s: %s) — there is no request path to estimate latency for", j.ID, flow.BlockedAt, flow.BlockedReason), prov).ToEnvelope()
	}

	est := JourneyLatencyEstimate{Components: []ComponentLatency{}, Assumptions: latencyAssumptions, Seed: LatencySeedNote}
	est.Inputs = append(est.Inputs,
		LatencyInput{Name: "journey " + j.ID + " peak_rps", Value: *j.PeakRPS, Source: "stated"},
		LatencyInput{Name: "journey " + j.ID + " steady_rps", Value: *j.SteadyRPS, Source: "stated"},
	)

	seenInput := map[string]bool{}
	sum := 0.0
	for _, members := range flow.ReachedByGroup {
		var worst *ComponentLatency
		for _, id := range members {
			if id == JourneyInternetSentinel {
				continue
			}
			l, ok := loads[id]
			if !ok || l.Capacity == nil {
				// Cannot happen after the readiness gate, but a defaulted capacity would
				// be the one unforgivable failure here, so it is checked, not trusted.
				return NotAssessable[any](fmt.Sprintf("component %q has no declared capacity — latency is never estimated from a guessed service rate", id), prov).ToEnvelope()
			}
			if !seenInput[l.CapacityKey] {
				seenInput[l.CapacityKey] = true
				est.Inputs = append(est.Inputs, LatencyInput{Name: "Workload.Capacity[" + l.CapacityKey + "]", Value: *l.Capacity, Source: "stated"})
			}
			scv, scvDeclared := workload.ServiceTimeSCV[l.CapacityKey]
			if scvDeclared && !seenInput["scv:"+l.CapacityKey] {
				seenInput["scv:"+l.CapacityKey] = true
				est.Inputs = append(est.Inputs, LatencyInput{Name: "Workload.ServiceTimeSCV[" + l.CapacityKey + "]", Value: scv, Source: "stated"})
			}
			c := componentLatency(l, scv, scvDeclared)
			if worst == nil || moreLatent(c, *worst) {
				cc := c
				worst = &cc
			}
		}
		if worst == nil {
			continue
		}
		est.Components = append(est.Components, *worst)
		if worst.Saturated && est.SaturatedAt == "" {
			est.SaturatedAt = worst.NodeID
		}
		if worst.MeanMS != nil {
			sum += *worst.MeanMS
		}
	}
	declared := est.Inputs[2:] // the two journey rates stay first; capacities sorted by name (NFR-1)
	sort.Slice(declared, func(a, b int) bool { return declared[a].Name < declared[b].Name })

	if est.SaturatedAt != "" {
		est.Statement = fmt.Sprintf("Under the declared inputs, journey %q is saturated at %s: its offered load meets or exceeds that component's declared capacity, so no steady-state latency exists (the queue grows without bound).", j.ID, est.SaturatedAt)
	} else {
		m := sum
		est.MeanMS = &m
		est.Statement = fmt.Sprintf("Under the declared inputs and the stated modelling assumptions, journey %q has a mean end-to-end latency of ~%.1f ms (queueing delay plus service time at each component).", j.ID, sum)
	}
	return Assessed[any](est, prov).ToEnvelope()
}

// moreLatent orders two path-position members: a saturated one beats any finite one,
// then higher mean wins, ties broken by node ID so output never depends on input order
// (NFR-1).
func moreLatent(a, b ComponentLatency) bool {
	switch {
	case a.Saturated != b.Saturated:
		return a.Saturated
	case a.Saturated:
		return a.NodeID < b.NodeID
	case *a.MeanMS != *b.MeanMS:
		return *a.MeanMS > *b.MeanMS
	}
	return a.NodeID < b.NodeID
}

// componentLatency is the queueing closed form. With service rate mu (declared capacity,
// rps) and arrival rate lambda < mu:
//
//   - no declared SCV (assumed exponential service, M/M/1): the sojourn time is exponential
//     with rate (mu - lambda), so mean = 1/(mu-lambda) and p95 = ln(20)/(mu-lambda);
//   - declared SCV (M/G/1): the Pollaczek-Khinchine mean, 1/mu + lambda*E[S^2]/(2*(1-rho))
//     with E[S^2] = (1+SCV)/mu^2. SCV = 1 reproduces M/M/1 exactly; SCV = 0 is M/D/1. No
//     p95 is given: its closed form needs exponential sojourn times.
func componentLatency(l ComponentLoad, scv float64, scvDeclared bool) ComponentLatency {
	mu, lambda := *l.Capacity, l.OfferedRPS
	c := ComponentLatency{NodeID: l.NodeID, OfferedRPS: lambda, CapacityRPS: mu, Utilization: lambda / mu, ServiceTimeSCV: 1, SCVDeclared: scvDeclared}
	if scvDeclared {
		c.ServiceTimeSCV = scv
	}
	if mu <= 0 || lambda >= mu {
		c.Saturated = true
		return c
	}
	if !scvDeclared {
		mean := 1000 / (mu - lambda)
		p95 := mean * math.Log(20)
		c.MeanMS, c.P95MS = &mean, &p95
		return c
	}
	rho := lambda / mu
	mean := 1000 * (1/mu + lambda*(1+scv)/(mu*mu)/(2*(1-rho)))
	c.MeanMS = &mean
	return c
}
