package core

// PC-131: saved Failure Lab scenarios. A scenario is a NAME and a list of declared
// faults — a DEFINITION, never a stored result. Every result is recomputed by
// core.Simulate against whichever version it is asked about, so a report built for
// version N shows what the scenario does to design N, not what it did to an older one
// (the Card's own rule: "re-run against each new version, not replayed from stored
// results").

// SavedScenario is one saved scenario definition.
type SavedScenario struct {
	Name   string  `json:"name" validate:"required,min=1" jsonschema:"required,minLength=1"`
	Faults []Fault `json:"faults" validate:"required,min=1,dive" jsonschema:"required,minItems=1"`
}

// ScenarioResult is a saved scenario evaluated NOW against one version's IR and workload.
type ScenarioResult struct {
	Name   string  `json:"name" validate:"required" jsonschema:"required"`
	Faults []Fault `json:"faults" validate:"required" jsonschema:"required"`
	// Verdict is core.Simulate's own verdict envelope — not_assessable (with its reason)
	// when a fault could not be resolved, e.g. its target no longer exists in this design.
	Verdict      AssessmentEnvelope `json:"verdict" validate:"required" jsonschema:"required"`
	SeveredPaths []string           `json:"severed_paths"`
	Cascade      []string           `json:"cascade"`
	// FailedJourneys/DegradedJourneys are the declared journeys that do not flow under the
	// scenario, split by whether the journey declared a fallback that does flow
	// (JourneyFlowResult.Degraded). Both empty when the workload declares no journeys.
	FailedJourneys   []string `json:"failed_journeys"`
	DegradedJourneys []string `json:"degraded_journeys"`
	// AlreadyBlockedJourneys (PC-161) are declared journeys that did not flow BEFORE the fault: they
	// are never counted as failed or survived by the scenario. UncheckedJourneys are those whose
	// baseline the engine could not decide. Both are empty when the workload declares no journeys.
	AlreadyBlockedJourneys []string `json:"already_blocked_journeys"`
	UncheckedJourneys      []string `json:"unchecked_journeys"`
}

// EvaluateScenarios runs every saved scenario against ir/workload, in the order given
// (callers pass them sorted by name, NFR-1). Pure: no I/O.
func EvaluateScenarios(ir *IR, workload Workload, saved []SavedScenario, prov Provenance) []ScenarioResult {
	out := make([]ScenarioResult, 0, len(saved))
	for _, sc := range saved {
		sim := Simulate(ir, workload, sc.Faults, prov)
		res := ScenarioResult{
			Name: sc.Name, Faults: sc.Faults, Verdict: sim.Verdict,
			SeveredPaths: nonNil(sim.SeveredPaths), Cascade: nonNil(sim.Cascade),
			FailedJourneys: []string{}, DegradedJourneys: []string{},
			AlreadyBlockedJourneys: []string{}, UncheckedJourneys: []string{},
		}
		// PC-161: classify from the baseline, which compares each journey before and after the fault,
		// so a journey that never flowed is not reported as having failed under this scenario.
		for _, b := range sim.Baseline.Journeys {
			switch b.Status {
			case BaselineBrokenByFault:
				res.FailedJourneys = append(res.FailedJourneys, b.JourneyID)
			case BaselineDegradedByFault:
				res.DegradedJourneys = append(res.DegradedJourneys, b.JourneyID)
			case BaselineAlreadyBlocked:
				res.AlreadyBlockedJourneys = append(res.AlreadyBlockedJourneys, b.JourneyID)
			case BaselineNotAssessable:
				res.UncheckedJourneys = append(res.UncheckedJourneys, b.JourneyID)
			}
		}
		out = append(out, res)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
