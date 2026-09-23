// Package contracts_test is deliberately its own module-level test package (not
// core_test) — this test's job is to prove the *generated schema files* validate real
// payloads, using a JSON Schema validator that has nothing to do with core's own Go
// types. Round-tripping through Go's json package and calling core.IR.Validate() would
// only prove the Go struct tags are self-consistent; it says nothing about whether
// contracts/ir.schema.json — the artifact PC-7 actually promises to freeze — is a
// correct JSON Schema document that a non-Go consumer could validate against.
package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// pairs is PC-7's fourth acceptance criterion, made concrete: each schema must
// validate successfully against at least one real sample payload — extended to
// canvas.schema.json (PC-86 groundwork) on the same standard the original five got.
var pairs = []struct {
	schema string
	sample string
}{
	{"ir.schema.json", "ir.sample.json"},
	{"provenance.schema.json", "provenance.sample.json"},
	{"workload.schema.json", "workload.sample.json"},
	{"finding.schema.json", "finding.sample.json"},
	{"finding.schema.json", "finding-compliance.sample.json"}, // PC-18: exercises EvidenceRef.Attribute
	{"adr.schema.json", "adr.sample.json"},
	{"canvas.schema.json", "canvas.sample.json"}, // PC-86 groundwork: real output captured from actually driving canvas/ in a live browser (Playwright), not hand-typed
}

func TestGeneratedSchemasValidateSamplePayloads(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	contractsDir := filepath.Join(root, "..", "contracts")

	for _, p := range pairs {
		p := p
		t.Run(p.schema, func(t *testing.T) {
			schemaPath := filepath.Join(contractsDir, p.schema)
			samplePath := filepath.Join(contractsDir, "samples", p.sample)

			c := jsonschema.NewCompiler()
			sch, err := c.Compile(schemaPath)
			if err != nil {
				t.Fatalf("compile %s: %v", p.schema, err)
			}

			sampleFile, err := os.Open(samplePath)
			if err != nil {
				t.Fatalf("open %s: %v", p.sample, err)
			}
			defer sampleFile.Close()

			var doc any
			dec := json.NewDecoder(sampleFile)
			dec.UseNumber() // matches what jsonschema itself expects for numeric keywords
			if err := dec.Decode(&doc); err != nil {
				t.Fatalf("unmarshal %s: %v", p.sample, err)
			}

			if err := sch.Validate(doc); err != nil {
				t.Fatalf("%s does NOT validate against %s:\n%v", p.sample, p.schema, err)
			}
		})
	}
}

// TestGeneratedSchemasRejectInvalidPayloads is the negative control proving the
// schemas are actually discriminating, not merely permissive enough to accept
// anything — a schema with "additionalProperties" left unset would pass the positive
// test above while rejecting nothing.
func TestGeneratedSchemasRejectInvalidPayloads(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	contractsDir := filepath.Join(root, "..", "contracts")

	c := jsonschema.NewCompiler()
	sch, err := c.Compile(filepath.Join(contractsDir, "provenance.schema.json"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	invalid := map[string]any{
		"value": 60,
		"provenance": map[string]any{
			"kind":   "guessed", // not one of the five enum values
			"source": "somewhere",
		},
	}
	if err := sch.Validate(invalid); err == nil {
		t.Fatal("expected validation error for an invalid Kind enum value, got nil")
	}

	missingRequired := map[string]any{
		"value": 60,
		// provenance omitted entirely
	}
	if err := sch.Validate(missingRequired); err == nil {
		t.Fatal("expected validation error for a missing required 'provenance' field, got nil")
	}
}
