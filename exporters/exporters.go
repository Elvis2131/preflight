// Package exporters generates FIS / Chaos Studio / Litmus experiment manifests and the
// FINOS CALM export (PC-23, PC-27).
package exporters

import "preflight/core"

// ExportedExperiment is one derived chaos-experiment artifact — PC-23's own
// Conversation, verbatim: "exported experiments are a derived artifact, not a
// finding — confirm they carry their own provenance tag distinct from the
// failure-mode findings that generated them." Provenance here is always Kind
// "derived", sourced to the exporter itself, never inheriting or aliasing the
// source Finding's own Provenance — the export is a NEW derived fact ("this
// finding can be tested this way"), not a restatement of the finding.
type ExportedExperiment struct {
	// Format names the target system this experiment runs against — "fis" (AWS
	// Fault Injection Simulator) or "litmus" (LitmusChaos, Kubernetes).
	Format string `json:"format"`

	// SourceFindingID names the Finding this experiment was derived from, by ID —
	// never embeds the Finding itself (that would blur "derived artifact" back into
	// "part of the finding").
	SourceFindingID string `json:"source_finding_id"`

	// Content is the complete, ready-to-submit experiment document: JSON for FIS
	// (matches `aws fis create-experiment-template --cli-input-json`'s own expected
	// input shape exactly), YAML for Litmus (a real ChaosEngine custom resource,
	// `kubectl apply -f`-ready).
	Content string `json:"content"`

	Provenance core.Provenance `json:"provenance"`
}
