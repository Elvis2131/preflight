// This file is PC-154's wire/storage vocabulary for LLM narratives. They are DATA ONLY and
// deliberately live in core so the assessment engine (P1) can store, replay and report them
// without ever importing reason/ (the LLM layer, P2): the process boundary of ADR-003 is a
// package boundary, enforced by a test. Nothing here computes, decides or mutates a verdict
// (I3): an annotation is a separate value beside a finding, never a field of one.
package core

// LLMAnnotationNotice is shown wherever annotations are displayed, so no reader mistakes
// model-written prose for an engine result.
const LLMAnnotationNotice = "Written by a language model from the findings above; it explains them and decides nothing. The findings, their statuses and every number are the engine's, not the model's."

// LLMAnnotation is narrative about ONE finding, produced by an LLM, citing the evidence it
// reasoned from (CLAUDE.md §12) and tagged llm_reasoned.
type LLMAnnotation struct {
	FindingID     string     `json:"finding_id"`
	Narrative     string     `json:"narrative"`
	CitedEvidence []string   `json:"cited_evidence"`
	Provenance    Provenance `json:"provenance"`
}

// LLM rejection kinds: why a narrative was not accepted.
const (
	// LLMRejectionCallFailed: the model call itself failed (network, rate limit, upstream
	// error). Counts toward a degraded run.
	LLMRejectionCallFailed = "call_failed"
	// LLMRejectionInvalid: the model answered but the narrative was discarded (not the
	// requested JSON, no/invented citations, too long).
	LLMRejectionInvalid = "invalid"
)

// LLMRejection records a narrative that was discarded and why, so a bad model run is
// visible rather than silently thinning the output.
type LLMRejection struct {
	FindingID string `json:"finding_id"`
	Reason    string `json:"reason"`
	Kind      string `json:"kind"`
}

// LLM annotation set statuses.
const (
	LLMStatusComplete = "complete" // every finding annotated
	LLMStatusDegraded = "degraded" // the narrative layer could not (fully) contribute; findings unaffected
)

// LLMAnnotationSet is the stored/streamed/reported result of annotating one version. Status
// "degraded" is NOT an error: the deterministic result is complete either way (NFR-11).
type LLMAnnotationSet struct {
	Status      string          `json:"status"`
	Reason      string          `json:"reason,omitempty"`
	Model       string          `json:"model,omitempty"`
	Annotations []LLMAnnotation `json:"annotations"`
	Rejected    []LLMRejection  `json:"rejected"`
	Notice      string          `json:"notice"`
}
