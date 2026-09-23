package ingest

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"preflight/core"
)

// LoadWorkload parses a workload.yaml file into core.Workload and validates it against
// the same struct tags contracts/workload.schema.json is generated from — a workload
// file that fails PC-7's own frozen schema is rejected here, not silently accepted and
// discovered broken three layers downstream.
func LoadWorkload(path string) (core.Workload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.Workload{}, fmt.Errorf("ingest: read workload file %s: %w", path, err)
	}

	var w core.Workload
	if err := yaml.Unmarshal(data, &w); err != nil {
		return core.Workload{}, fmt.Errorf("ingest: parse workload file %s: %w", path, err)
	}
	if err := w.Validate(); err != nil {
		return core.Workload{}, fmt.Errorf("ingest: %s does not satisfy the frozen workload schema: %w", path, err)
	}
	return w, nil
}
