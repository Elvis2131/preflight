package templates_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"preflight/core"
	"preflight/golden/templates"
	awsprovider "preflight/providers/aws"
	"preflight/server"
)

func registry(t *testing.T) awsprovider.Registry {
	t.Helper()
	r, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	return r
}

func mustIDs(t *testing.T) []string {
	t.Helper()
	ids, err := templates.IDs()
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// PC-108 criterion 1: at least three templates exist, each loadable.
func TestTemplates_AtLeastThree_AllLoadable(t *testing.T) {
	ids := mustIDs(t)
	if len(ids) < 3 {
		t.Fatalf("got %d templates %v, want at least 3", len(ids), ids)
	}
	for _, id := range ids {
		tpl, err := templates.Load(id)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		if tpl.Meta.ID != id || tpl.Meta.Name == "" || tpl.Meta.Description == "" {
			t.Errorf("%s: incomplete meta %+v", id, tpl.Meta)
		}
		if err := tpl.Canvas.Validate(); err != nil {
			t.Errorf("%s: canvas does not satisfy canvas.schema.json: %v", id, err)
		}
		if err := tpl.Workload.Validate(); err != nil {
			t.Errorf("%s: workload does not satisfy workload.schema.json: %v", id, err)
		}
		if vs := core.ValidateCanvasPlacement(tpl.Canvas); len(vs) != 0 {
			t.Errorf("%s: a template must be valid placement, got %+v", id, vs)
		}
		for _, n := range tpl.Canvas.Nodes {
			if _, ok := tpl.Layout[n.ID]; !ok {
				t.Errorf("%s: node %q has no layout entry", id, n.ID)
			}
		}
	}
}

// PC-108 criterion 2: checked-in IR, findings and failure-mode fixtures are byte-compared
// against a fresh derivation from the template's inputs. A failure here means either a
// template or the engine changed: hand-verify, then `go run ./cmd/gen-template-fixtures`.
func TestTemplates_FixturesAreByteIdenticalToAFreshDerivation(t *testing.T) {
	reg := registry(t)
	for _, id := range mustIDs(t) {
		d, err := templates.Derive(id, reg)
		if err != nil {
			t.Fatalf("derive %s: %v", id, err)
		}
		for name, got := range map[string][]byte{"ir": d.IR, "findings": d.Findings, "failure": d.Failure} {
			want, err := templates.Raw(id, name)
			if err != nil {
				t.Fatalf("%s: missing fixture %s.json: %v", id, name, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s/%s.json differs from a fresh derivation — hand-verify the change, then regenerate with `go run ./cmd/gen-template-fixtures`", id, name)
			}
		}
		if len(d.Failure) == 0 {
			t.Errorf("%s has no failure-mode result", id)
		}
	}
}

// PC-108 criterion 3, the in-test negative control: change one template input and the
// derivation must no longer match the checked-in fixture — so the byte comparison above
// is sensitive to exactly the drift it exists to catch.
func TestTemplates_TamperedInputNoLongerMatchesFixture(t *testing.T) {
	reg := registry(t)
	tpl, err := templates.Load("three-tier-vpc")
	if err != nil {
		t.Fatal(err)
	}
	for i := range tpl.Canvas.Nodes {
		if tpl.Canvas.Nodes[i].ID == "aws_db_instance.db" {
			tpl.Canvas.Nodes[i].Capability = map[string]string{"storage_encrypted": "false", "multi_az": "true"}
		}
	}
	d, err := templates.DeriveFrom(tpl, reg)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := templates.Raw("three-tier-vpc", "findings")
	if bytes.Equal(d.Findings, want) {
		t.Fatal("un-encrypting the database did not change the derived findings — the fixture comparison cannot detect drift")
	}
}

// PC-108 criterion 4: loading a template is the SAME pipeline as a hand-drawn design.
// The template's canvas document and workload go through the unmodified
// server.AssessCanvas, and what comes back is the checked-in findings fixture, byte for
// byte — there is no template-specific path for it to have taken.
func TestTemplates_AssessThroughTheOrdinaryCanvasPipeline(t *testing.T) {
	for _, id := range mustIDs(t) {
		tpl, err := templates.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		store, err := server.OpenStore(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		resp, err := server.AssessCanvas(store, server.AssessCanvasRequest{SessionID: "tpl-" + id, Canvas: tpl.Canvas, Workload: &tpl.Workload})
		store.Close()
		if err != nil {
			t.Fatalf("%s: AssessCanvas: %v", id, err)
		}
		got, err := json.MarshalIndent(resp.Findings, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		want, _ := templates.Raw(id, "findings")
		if !bytes.Equal(got, want) {
			t.Errorf("%s: findings from the ordinary AssessCanvas path differ from the checked-in fixture", id)
		}
	}
}

// Every checked-in file under golden/templates/<id>/ is one the library knows about —
// a stray or misspelled fixture would otherwise sit there unverified.
func TestTemplates_NoUnknownFiles(t *testing.T) {
	known := map[string]bool{"meta.json": true, "canvas.json": true, "workload.json": true, "layout.json": true, "ir.json": true, "findings.json": true, "failure.json": true}
	for _, id := range mustIDs(t) {
		entries, err := os.ReadDir(filepath.Join(".", id))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !known[e.Name()] {
				t.Errorf("%s/%s is not a known template file", id, e.Name())
			}
		}
	}
}
