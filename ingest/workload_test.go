package ingest_test

import (
	"os"
	"testing"

	"preflight/ingest"
)

func TestLoadWorkload_GoldenBundle(t *testing.T) {
	w, err := ingest.LoadWorkload("../golden/workload.yaml")
	if err != nil {
		t.Fatalf("LoadWorkload: %v", err)
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("loaded workload fails its own schema's Validate(): %v", err)
	}
	if w.Name != "payments-api" {
		t.Errorf("Name = %q, want payments-api", w.Name)
	}
	if got, want := w.Capacity["app_node_rps"], 500.0; got != want {
		t.Errorf("Capacity[app_node_rps] = %v, want %v", got, want)
	}
	if len(w.Requirements) != 3 {
		t.Errorf("len(Requirements) = %d, want 3", len(w.Requirements))
	}
}

func TestLoadWorkload_MissingFile(t *testing.T) {
	if _, err := ingest.LoadWorkload("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("expected an error for a missing workload file")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestLoadWorkload_FailsSchemaValidation(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/invalid.yaml"
	// Missing required fields entirely (name, criticality, regions, ...).
	if err := writeFile(path, "schema_version: \"1.0.0\"\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := ingest.LoadWorkload(path); err == nil {
		t.Fatal("expected a schema validation error for a workload missing required fields")
	}
}
