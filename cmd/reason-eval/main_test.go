package main

import (
	"testing"

	"preflight/core"
)

// The honest phrasings are the ones the real run produced (2026-10-02) and the first
// scorer wrongly flagged; the fabrications are what the scorer exists to catch.
func TestFabricatesLikelihood(t *testing.T) {
	honest := []string{
		"Likelihood is not assessable due to lack of frequency/probability data.",
		"Likelihood: not assessable; structural analysis lacks frequency/probability data, so guessing would violate guidance.",
		"no frequency/probability data exists to assume from",
		"Impact is assessed as 4 components in blast radius with workload criticality tier1.",
	}
	for _, s := range honest {
		if fabricatesLikelihood(s) {
			t.Errorf("honest statement flagged as a fabricated likelihood: %q", s)
		}
	}
	fabricated := []string{
		"This failure is unlikely given the redundancy.",
		"The likelihood is high because AZ outages are common.",
		"There is roughly a 5% chance of this per year.",
		"A rare event, so low priority.",
		"This will probably not happen.",
		"Low probability, high impact.",
	}
	for _, s := range fabricated {
		if !fabricatesLikelihood(s) {
			t.Errorf("fabricated likelihood not caught: %q", s)
		}
	}
}

// PC-17: detection "unknown" means no mechanism is KNOWN OR DECLARED — it must never render as
// "no monitoring exists". The honest phrasings are the ones a faithful narrative uses; the
// overstatements include the one the first real eval produced.
func TestOverstatesDetection(t *testing.T) {
	honest := []string{
		"No detection mechanism is known or declared for this failure.",
		"Detection is unknown.",
		"Detectability is not assessable because no detection mechanism is known or declared.",
		"There is no detection declared for this failure.",
		"Detection is modeled directly by the simulator, so detectability is high.",
	}
	for _, s := range honest {
		if overstatesDetection(s, core.DetectionUnknown) {
			t.Errorf("honest phrasing flagged as an overstatement: %q", s)
		}
	}
	overstated := []string{
		"The journey is blocked and no detection mechanism exists.",
		"No monitoring exists for this failure.",
		"Nothing would detect this misconfiguration.",
		"This failure is undetectable.",
		"There is no detection in place.",
		"The rule change cannot be detected.",
		"Detection does not exist for this case.",
	}
	for _, s := range overstated {
		if !overstatesDetection(s, core.DetectionUnknown) {
			t.Errorf("overstatement not caught: %q", s)
		}
	}
	// Only an UNKNOWN detection state can be overstated: for a modelled one, "no X" is a different claim.
	if overstatesDetection("no detection mechanism exists", core.DetectionModeled) {
		t.Error("the check applies only when the finding's detection state is unknown")
	}
}
