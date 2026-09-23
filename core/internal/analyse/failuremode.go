// This file derives the three dimensions PC-14's existing engines don't already
// answer (Impact, Likelihood, Detectability) — Recoverability reuses PC-14's
// RTOFeasibility/RPOFeasibility directly and needs no new derivation here (see
// core/finding.go's Recoverability doc comment).
//
// Same package rule as everywhere else in this file: no dependency on preflight/core.
package analyse

import "fmt"

// DeriveLikelihood answers PC-17's most consequential Conversation question directly:
// "Likelihood will be unknown in almost every static-analysis case — is that
// acceptable? Answer per the PRD: yes — unknown beats fabricated." This function
// therefore ALWAYS returns not-assessable for Layer 1 structural analysis: there is no
// frequency/probability data anywhere in a Terraform bundle or a workload declaration
// to assume from, and assuming one would BE the fabrication CLAUDE.md §10 names
// explicitly ("assumed | unknown — never fabricated; usually unknown"). This is not a
// placeholder pending a smarter implementation — it is the CORRECT, permanent answer
// for this analysis layer. A future Rung-3/observed data source (P2) could someday
// supply a real value, which is why this is its own function rather than a literal
// not-assessable hardcoded at every call site.
func DeriveLikelihood() (value string, ok bool, notAssessableReason string) {
	return "", false, "likelihood is not assessable from structural analysis alone — no frequency/probability data exists to assume from; an assumed guess here would be exactly the fabrication CLAUDE.md §10 warns against"
}

// DeriveDetectability derives from Detection exactly as CLAUDE.md §10 specifies
// ("detectability: from the detection field") — a categorical, non-numeric
// description, never a score (see PC-17's own first acceptance criterion: no code
// path may produce a single combined severity number, and that discipline extends to
// each individual dimension too, not just their combination).
func DeriveDetectability(detection string) (value string, ok bool, notAssessableReason string) {
	switch detection {
	case "modeled":
		return "high — this simulator models the failure directly", true, ""
	case "observed":
		return "high — confirmed via a real observed experiment (validation ladder Rung 1/3)", true, ""
	case "declared":
		return "medium — detection depends on a declared, unverified mechanism (e.g. a runbook or alert)", true, ""
	case "unknown":
		return "", false, "no detection mechanism is known or declared for this failure"
	default:
		return "", false, fmt.Sprintf("detection state %q has no known detectability rule", detection)
	}
}

// DeriveImpact is CLAUDE.md §10's "impact (derived: blast radius × workload
// criticality)" — deliberately a descriptive STRING, never an arithmetic product
// masquerading as "×": there is no numeric scale for "workload criticality" or "blast
// radius" this codebase has defined, and inventing one to literally multiply would be
// exactly the composite-severity-number anti-pattern PC-17's first acceptance
// criterion forbids. "×" in the source text names the two INPUTS this dimension
// considers together, not a literal multiplication.
func DeriveImpact(blastRadiusSize int, workloadCriticality string) (value string, ok bool, notAssessableReason string) {
	if blastRadiusSize == 0 {
		return "", false, "blast radius is empty — either the failure is fully contained/redundant, or blast radius could not be computed"
	}
	if workloadCriticality == "" {
		return "", false, "workload criticality is not declared"
	}
	return fmt.Sprintf("%d component(s) in blast radius; workload criticality %q", blastRadiusSize, workloadCriticality), true, ""
}
