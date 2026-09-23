// Package aws holds the golden architecture's AWS resource-to-IR mappings as data
// (Design §4.5 concept, PC-13): one YAML file per resource type under this directory.
// The mapping SCHEMA itself (ResourceMapping, CapabilityMapping, FailoverMapping,
// Registry, etc.) lives in the parent providers/ package (PC-22: extracted so aws/ and
// azure/ share one schema, not two that could drift) — this file only embeds AWS's own
// YAML data and loads it through that shared schema. No AWS behavior, no capability
// VALUES, and no per-resource logic live here — that is what would make this "code,
// not data," which PC-13's own Conversation explicitly warns against.
package aws

import (
	"embed"

	"preflight/providers"
)

//go:embed *.yaml
var mappingFiles embed.FS

// Re-exported so existing call sites (ingest, core, tests) that spell these as
// awsprovider.ResourceMapping etc. keep working unchanged — this package remains a
// valid, complete alias of the shared schema, not a second copy of it.
type (
	OnAbsent          = providers.OnAbsent
	CapabilityMapping = providers.CapabilityMapping
	FailoverOutcome   = providers.FailoverOutcome
	FailoverMapping   = providers.FailoverMapping
	EdgeMapping       = providers.EdgeMapping
	ResourceMapping   = providers.ResourceMapping
	Registry          = providers.Registry
)

const (
	OnAbsentNotAssessable = providers.OnAbsentNotAssessable
	OnAbsentAssumed       = providers.OnAbsentAssumed
)

// Load reads and validates every *.yaml file embedded in this package.
func Load() (Registry, error) {
	return providers.Load(mappingFiles, "providers/aws")
}
