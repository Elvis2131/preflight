package main

import "testing"

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
