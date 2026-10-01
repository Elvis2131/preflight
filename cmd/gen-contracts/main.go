// Command gen-contracts generates contracts/*.schema.json from the Go structs in
// package core, via invopop/jsonschema. PC-7's acceptance criterion is explicit: each
// schema is generated from the same Go structs the code validates against — not
// hand-written JSON. This command is that generation step; contracts/CHANGELOG.md
// documents the versioning convention for the frozen output it produces.
//
// Run with: go run ./cmd/gen-contracts
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/invopop/jsonschema"

	"preflight/core"
)

// Per-contract versions. Design §5 requires each schema be "versioned independently" —
// an earlier version of this generator stamped one global constant on all five, which
// could never actually express that (a change to one schema had no way to bump only
// that one's version). Fixed here, prompted by PC-17 giving finding.schema.json its
// first real breaking change since the PC-7 freeze: only finding.schema.json's version
// moves; the other four are untouched by this change and correctly stay at 1.0.0.
type contract struct {
	file    string
	typ     any
	version string
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-contracts:", err)
		os.Exit(1)
	}
	outDir := filepath.Join(root, "contracts")

	contracts := []contract{
		{"ir.schema.json", core.IR{}, "1.4.0"},
		{"provenance.schema.json", core.TaggedEnvelope{}, "1.0.0"},
		{"workload.schema.json", core.Workload{}, "1.4.0"}, // PC-131: DeclaredJourney.fallback — see CHANGELOG.md
		{"finding.schema.json", core.Finding{}, "1.2.0"},   // PC-18: EvidenceRef.Attribute added — see CHANGELOG.md
		{"adr.schema.json", adrAndWaiver{}, "1.0.0"},
		{"canvas.schema.json", core.CanvasDocument{}, "1.6.0"}, // PC-86 groundwork: sixth frozen contract — see CHANGELOG.md
		{"report.schema.json", core.Report{}, "1.6.0"},         // PC-120: seventh frozen contract — see CHANGELOG.md
	}

	for _, c := range contracts {
		if err := generate(outDir, c); err != nil {
			fmt.Fprintf(os.Stderr, "gen-contracts: %s: %v\n", c.file, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", filepath.Join("contracts", c.file))
	}
}

// adrAndWaiver exists only so adr.schema.json can hold both frozen types Design §5
// assigns to that one file ("ADR & waiver | adr.schema.json | Decision records, risk
// acceptance, expiry") — invopop/jsonschema reflects one root type per call, and ADR/
// Waiver are two independent, not-nested types, so this wrapper is the schema-generation
// mechanism, not a claim that an ADR document embeds a Waiver or vice versa.
type adrAndWaiver struct {
	ADR    core.ADR    `json:"adr"`
	Waiver core.Waiver `json:"waiver"`
}

func generate(outDir string, c contract) error {
	r := &jsonschema.Reflector{
		DoNotReference: false,
		ExpandedStruct: true,
	}
	if err := r.AddGoComments("preflight/core", "./core"); err != nil {
		// Comments are a documentation nicety (surfaced as JSON Schema "description"),
		// not correctness — degrade to schemas without them rather than fail the build.
		fmt.Fprintf(os.Stderr, "gen-contracts: warning: AddGoComments: %v\n", err)
	}

	schema := r.ReflectFromType(reflect.TypeOf(c.typ))
	schema.Version = "https://json-schema.org/draft/2020-12/schema"
	if schema.Extras == nil {
		schema.Extras = map[string]any{}
	}
	schema.Extras["x-schema-version"] = c.version
	schema.Extras["x-changelog"] = "contracts/CHANGELOG.md"

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	buf = append(buf, '\n')

	return os.WriteFile(filepath.Join(outDir, c.file), buf, 0o644)
}
