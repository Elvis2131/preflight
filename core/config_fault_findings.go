// This file is PC-130's own acceptance criterion, verbatim: "Fault results appear as
// findings with correct four-dimension values and flow into delta/report." One
// finding per declared journey, built directly from ComputeConfigurationBlastSurface
// (this ticket, above) — never a per-applied-fault finding (a fault is something an
// architect deliberately injects via /simulate to explore one scenario; this is the
// structural fragility fact "how many single rule changes would break this journey,"
// computed once per version, the same "design-time fragility" shape
// BuildNATSharedAcrossAZsFindings (PC-129) already established for a different fault
// class).
//
// Detectability, per the Card's own explicit instruction: "detectability is usually
// unknown (a config change isn't detected by redundancy), which is correct and
// should not be defaulted to anything better." Detection is set to DetectionUnknown
// here, unconditionally — DeriveDetectability(DetectionUnknown, ...) then correctly
// resolves to not_assessable ("no detection mechanism is known or declared"), not a
// guessed "low"/"medium" value.
package core

import "strconv"

// BuildConfigurationBlastSurfaceFindings runs ComputeConfigurationBlastSurface for
// every declared journey and reports its blast surface: how many of the real rules on
// its own path would break it if changed alone.
//
// It is DESCRIPTIVE, not a verdict (corrected under PC-152). An earlier version marked a
// journey Unsatisfied whenever any single rule was load-bearing — but in a
// least-privilege design every rule that is the only one admitting a hop is load-bearing,
// so every working journey was "unsatisfied" and the agent-iteration acceptance test (a
// target of "all hard requirements satisfied") became unreachable. The Card asks for
// "the set of single rule changes that would break it", derived information in the same
// shape as the zone-kill findings ("2 component(s) affected"); whether a given
// fragility matters is the architect's judgement, never a pass/fail here. It is
// NotAssessable only when the journey does not structurally flow at all today (a blast
// surface is undefined without a working baseline to measure against).
func BuildConfigurationBlastSurfaceFindings(ir *IR, workload Workload) []Finding {
	var findings []Finding
	for _, j := range workload.Journeys {
		findings = append(findings, configurationBlastSurfaceFinding(ir, j))
	}
	return findings
}

func configurationBlastSurfaceFinding(ir *IR, j DeclaredJourney) Finding {
	prov := NewProvenance(KindDerived, "core/config_blast_surface:"+j.ID)

	baseline := ComputeJourneyFlow(ir, j, nil)
	notAssessable := false
	var evidence []EvidenceRef
	var value string
	if !baseline.Flows {
		notAssessable = true
		evidence = []EvidenceRef{{Description: "journey \"" + j.ID + "\" does not structurally flow today (" + baseline.BlockedReason + ") — a configuration blast surface is not meaningful without a working baseline"}}
	} else {
		surface := ComputeConfigurationBlastSurface(ir, j, nil)
		capNote := ""
		if surface.Capped {
			capNote = " (search capped at " + strconv.Itoa(ConfigBlastSurfaceCap) + " candidate rules — more may exist)"
		}
		if len(surface.BreakingChanges) > 0 {
			value = strconv.Itoa(len(surface.BreakingChanges)) + " of " + strconv.Itoa(surface.RulesConsidered) + " real rules on this journey's path would break it if changed alone" + capNote
			evidence = journeyBlastSurfaceEvidence(surface)
		} else {
			value = "no single rule change among the " + strconv.Itoa(surface.RulesConsidered) + " real rules on this journey's path breaks it" + capNote
			evidence = []EvidenceRef{{Description: value}}
		}
	}

	var notAssessableReason string
	if notAssessable {
		notAssessableReason = evidence[0].Description
	}
	outcome := AssessmentEnvelope{State: AssessmentStateAssessed, Value: value, Provenance: prov}
	if notAssessable {
		outcome = NotAssessable[any](notAssessableReason, prov).ToEnvelope()
	}

	return Finding{
		ID:    "finding.resilience.configuration-blast-surface." + j.ID,
		Title: "Journey \"" + j.Name + "\": configuration blast surface (single rule changes that would break it)",
		Dimensions: FailureMode{
			Trigger:            "a single security-group or NACL rule is tightened, removed, or misconfigured — infrastructure intact, a rule changed",
			AffectedComponents: j.Path,
			Detection:          DetectionUnknown,
			Impact:             NotAssessable[any]("impact dimension not evaluated by this check", prov).ToEnvelope(),
			Likelihood:         DeriveLikelihood(prov).ToEnvelope(),
			Detectability:      DeriveDetectability(DetectionUnknown, prov).ToEnvelope(),
			Recoverability: Recoverability{
				FailoverPathExists: NotAssessable[any]("recoverability not evaluated by this check", prov).ToEnvelope(),
				RPOFeasible:        NotAssessable[any]("recoverability not evaluated by this check", prov).ToEnvelope(),
			},
		},
		Evidence: evidence,
		Outcome:  outcome,
	}
}

// journeyBlastSurfaceEvidence produces one EvidenceRef per real breaking rule,
// correctly attributed to the actual SG/NACL node it lives on (never a synthetic
// "journey" node ID — EvidenceRef.NodeID names a real IR node, and a journey is not
// one), plus a summary EvidenceRef naming the total/cap. Finding.Evidence requires at
// least one entry; this always produces at least the summary.
func journeyBlastSurfaceEvidence(surface ConfigBlastSurfaceResult) []EvidenceRef {
	summary := strconv.Itoa(len(surface.BreakingChanges)) + " of " + strconv.Itoa(surface.RulesConsidered) + " real rules on this journey's own path, if removed alone, would break it"
	if surface.Capped {
		summary += " (search capped at " + strconv.Itoa(ConfigBlastSurfaceCap) + " candidate rules — more may exist)"
	}
	evidence := []EvidenceRef{{Description: summary}}

	sgAttr := "security_group_rules"
	naclAttr := "nacl_rules"
	for _, entry := range surface.BreakingChanges {
		if entry.SGRule != nil {
			nodeID := entry.SGNodeID
			evidence = append(evidence, EvidenceRef{
				NodeID: &nodeID, Attribute: &sgAttr,
				Description: "removing this rule (" + entry.SGRule.Direction + " " + entry.SGRule.Protocol + ") alone breaks the journey",
			})
		} else {
			nodeID := entry.NACLNodeID
			evidence = append(evidence, EvidenceRef{
				NodeID: &nodeID, Attribute: &naclAttr,
				Description: "removing this rule (" + entry.NACLRule.Direction + " " + entry.NACLRule.Protocol + ") alone breaks the journey",
			})
		}
	}
	return evidence
}
