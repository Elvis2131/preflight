// Package azure holds the golden architecture's Azure resource-to-IR mappings as data
// (Design §4.5, PC-22) — the second provider proving the two-level IR abstraction
// actually generalizes, not just AWS with a different vocabulary pasted on. Same
// pattern as providers/aws: one YAML file per resource type, the shared schema
// (ResourceMapping, CapabilityMapping, FailoverMapping, Registry) lives in the parent
// providers/ package, this file only embeds Azure's own YAML data and loads it through
// that shared schema.
package azure

import (
	"embed"

	"preflight/providers"
)

//go:embed *.yaml
var mappingFiles embed.FS

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
	return providers.Load(mappingFiles, "providers/azure")
}
