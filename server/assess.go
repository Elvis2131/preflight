// This file is PC-21's core: Assess is the ONE function both the HTTP route and the
// MCP tool call (handlers wrap it, never reimplement it — this ticket's own
// Conversation names duplicated logic here as the exact risk to guard against).
package server

import (
	"fmt"
	"net/http"
	"time"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
	awsprovider "preflight/providers/aws"
	azureprovider "preflight/providers/azure"
	"preflight/render"
)

// AssessRequest is what a caller (an HTTP body or an MCP tool call — see http.go/
// mcp.go for how each decodes into this) provides to run one assessment.
type AssessRequest struct {
	// SessionID identifies the ongoing author->evaluate->modify->re-evaluate loop
	// (CLAUDE.md §3). A new SessionID starts Version 1; an existing one produces the
	// next VersionNumber and a real AssuranceDelta against its predecessor.
	SessionID string `json:"session_id"`

	// BundleDir is a directory of .tf files to ingest — the same shape golden/aws and
	// golden/aws-broken already are. A real "upload arbitrary Terraform" ingestion
	// path is not this ticket's scope; PC-21's Card is about the assurance LOOP
	// (latency, degradation, persistence), not about broadening ingest's own input
	// surface beyond what PC-12 already built.
	BundleDir string `json:"bundle_dir"`

	// WorkloadPath is the workload.yaml to load alongside BundleDir.
	WorkloadPath string `json:"workload_path"`
}

// AssessResponse is /assess's full response shape (PRD §6, Design §2.1).
type AssessResponse struct {
	SessionID     string         `json:"session_id"`
	VersionNumber int            `json:"version_number"`
	Findings      []core.Finding `json:"findings"`
	Scorecard     core.Scorecard `json:"scorecard"`
	// AssuranceDelta is nil for a session's first version (nothing to diff against —
	// core.DeltaEntry's own "removed finding" honesty applies at the whole-response
	// level here too: no prior version means no claim about direction is possible).
	AssuranceDelta []core.DeltaEntry `json:"assurance_delta,omitempty"`

	// Graph is PC-21's explicit, named scope exclusion (this ticket's own
	// Conversation): Design §4.10's SVG rendering has no story until PC-81. Always
	// present, always this exact stub value — never omitted, never an error — so a
	// caller can distinguish "not built yet" from "the field doesn't exist" or "this
	// request failed."
	Graph *string `json:"graph"`

	// Degraded is true whenever the LLM narrative layer (reason/) did not or could
	// not contribute — see reason.go's own doc comment for why this is unconditionally
	// true today (reason/ has no client at all yet, ADR-005/PC-77).
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degraded_reason,omitempty"`

	// ComputeDurationMS is measured, not estimated — PC-21's own first acceptance
	// criterion is about a real measured latency, and a response that couldn't state
	// its own duration would be asserting the criterion rather than demonstrating it.
	ComputeDurationMS int64 `json:"compute_duration_ms"`
}

// Assess runs one full deterministic assessment: ingest -> core engines (PC-14/17/18)
// -> scorecard -> (if a prior version exists) Assurance Delta (PC-19) -> persist
// (store.go) -> respond. It NEVER calls reason/ — see the Degraded field's doc
// comment — so the "under 5s, before the LLM call completes" criterion is met by
// construction: there is no LLM call in this path to wait on at all yet.
func Assess(store *Store, req AssessRequest) (AssessResponse, error) {
	start := time.Now()

	if req.SessionID == "" {
		return AssessResponse{}, newAPIError(http.StatusBadRequest, "invalid_request_body", "server: session_id is required")
	}

	versionNumber, hadPrev, prev, err := nextVersion(store, req.SessionID)
	if err != nil {
		return AssessResponse{}, err
	}

	// PC-29: merged, not AWS-only — a caller's bundle can be either cloud (or, in
	// principle, mix resource types from both), and ingest itself has no notion of
	// "which provider" beyond resource_type string matching (see providers.Merge's own
	// doc comment for why this merge is safe and unambiguous).
	awsRegistry, err := awsprovider.Load()
	if err != nil {
		return AssessResponse{}, fmt.Errorf("server: load AWS provider mappings: %w", err)
	}
	azureRegistry, err := azureprovider.Load()
	if err != nil {
		return AssessResponse{}, fmt.Errorf("server: load Azure provider mappings: %w", err)
	}
	registry := providers.Merge(awsRegistry, azureRegistry)
	workload, err := ingest.LoadWorkload(req.WorkloadPath)
	if err != nil {
		return AssessResponse{}, newAPIError(http.StatusUnprocessableEntity, "invalid_workload", "server: load workload: %s", err)
	}
	result, err := ingest.Ingest(req.BundleDir, registry, versionNumber)
	if err != nil {
		return AssessResponse{}, newAPIError(http.StatusUnprocessableEntity, "invalid_bundle", "server: ingest: %s", err)
	}

	return assessFromResult(store, req.SessionID, versionNumber, hadPrev, prev, workload, result, start)
}

// nextVersion is the session/version bookkeeping shared by every IR producer this
// server has (Assess's HCL path, AssessCanvas's canvas path, PC-86) — extracted so
// that bookkeeping, not the ingestion mechanism, is the one place a future producer
// needs to touch.
func nextVersion(store *Store, sessionID string) (versionNumber int, hadPrev bool, prev StoredVersion, err error) {
	if err = store.EnsureSession(sessionID); err != nil {
		return 0, false, StoredVersion{}, err
	}
	prev, hadPrev, err = store.LatestVersion(sessionID)
	if err != nil {
		return 0, false, StoredVersion{}, err
	}
	versionNumber = 1
	if hadPrev {
		versionNumber = prev.VersionNumber + 1
	}
	return versionNumber, hadPrev, prev, nil
}

// assessFromResult is the pipeline shared by every IR producer once it has a real
// ingest.Result in hand: core engines (PC-14/17/18) -> scorecard -> (if a prior
// version exists) Assurance Delta (PC-19) -> persist -> respond. Neither Assess nor
// AssessCanvas duplicates any of this — PC-21's own Conversation names duplicated
// logic here as the exact risk to guard against, and PC-86 extends that same
// discipline to the second producer rather than growing a second copy alongside it.
func assessFromResult(store *Store, sessionID string, versionNumber int, hadPrev bool, prev StoredVersion, workload core.Workload, result ingest.Result, start time.Time) (AssessResponse, error) {
	if result.Insufficient != nil {
		return AssessResponse{}, newAPIError(http.StatusUnprocessableEntity, "insufficient_model",
			"server: bundle below the Minimum Viable Graph threshold: %+v", result.Insufficient)
	}

	findings := core.BuildFindings(result.IR, workload)
	scorecard := core.BuildScorecard(findings, versionNumber)

	var delta []core.DeltaEntry
	if hadPrev {
		prov := core.NewProvenance(core.KindDerived, "server:assess:assurance-delta")
		delta = core.ComputeDelta(prev.Scorecard.StatusMap(), scorecard.StatusMap(), prov)
	}

	if err := store.StoreVersion(StoredVersion{
		SessionID: sessionID, VersionNumber: versionNumber,
		IR: result.IR, Findings: findings, Scorecard: scorecard, Workload: workload,
	}); err != nil {
		return AssessResponse{}, err
	}

	// Graph (PC-81): core.RenderDOT (pure, deterministic) -> render.SVG (the one place
	// this codebase shells out to Graphviz). A render failure — Graphviz not
	// installed being the realistic case — never fails the whole assessment
	// (findings/scorecard are the load-bearing result); it produces an honest,
	// specific message instead, the same "never omitted, never silently guessed"
	// contract this field has always had, now describing a real attempted-and-failed
	// state rather than "not built yet".
	graph := graphOrFailureMessage(result.IR)

	return AssessResponse{
		SessionID:         sessionID,
		VersionNumber:     versionNumber,
		Findings:          findings,
		Scorecard:         scorecard,
		AssuranceDelta:    delta,
		Graph:             &graph,
		Degraded:          true,
		DegradedReason:    "reason/ has no LLM client wired up yet (ADR-005/PC-77) — every response is narrative-degraded until it does",
		ComputeDurationMS: time.Since(start).Milliseconds(),
	}, nil
}

// graphOrFailureMessage renders result.IR to SVG (PC-81) or, if that fails, returns a
// specific message naming why — never a blank string, never a silently swallowed
// error.
func graphOrFailureMessage(ir *core.IR) string {
	svg, err := render.SVG(core.RenderDOT(ir))
	if err != nil {
		return "graph rendering failed: " + err.Error()
	}
	return svg
}
