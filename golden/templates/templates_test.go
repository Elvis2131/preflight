package templates_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"preflight/core"
	"preflight/golden/templates"
	"preflight/ingest"
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

// PC-137/138/139 end to end: the 3-tier template's journeys flow through the SG, NACL and
// route-table steps, authored entirely on the canvas — and each control is LOAD-BEARING:
// breaking exactly one of them makes the journey that depends on it fail at exactly that
// step, not somewhere else and not silently.
func TestThreeTier_JourneysFlow_AndEachNetworkControlIsLoadBearing(t *testing.T) {
	reg := registry(t)
	load := func() templates.Template {
		tpl, err := templates.Load("three-tier-vpc")
		if err != nil {
			t.Fatal(err)
		}
		return tpl
	}
	flows := func(tpl templates.Template) map[string]core.JourneyFlowResult {
		res, err := ingest.IngestCanvas(tpl.Canvas, reg, 1)
		if err != nil || res.IR == nil {
			t.Fatalf("ingest: %v / %+v", err, res.Insufficient)
		}
		out := map[string]core.JourneyFlowResult{}
		for _, j := range tpl.Workload.Journeys {
			out[j.ID] = core.ComputeJourneyFlow(res.IR, j, nil)
		}
		return out
	}

	base := flows(load())
	if len(base) != 3 {
		t.Fatalf("want 3 declared journeys, got %d", len(base))
	}
	for id, f := range base {
		if !f.Flows {
			t.Fatalf("baseline: journey %q must flow end to end, blocked at %s: %s", id, f.BlockedAt, f.BlockedReason)
		}
	}

	mutate := func(nodeID string, fn func(n *core.CanvasNode)) templates.Template {
		tpl := load()
		for i := range tpl.Canvas.Nodes {
			if tpl.Canvas.Nodes[i].ID == nodeID {
				fn(&tpl.Canvas.Nodes[i])
				return tpl
			}
		}
		t.Fatalf("no node %q", nodeID)
		return tpl
	}

	cases := []struct {
		name    string
		tpl     templates.Template
		journey string
		step    string
	}{
		{"remove the only rule letting the app reach the database (SG)",
			mutate("aws_security_group.db", func(n *core.CanvasNode) {
				n.SecurityGroupRules = n.SecurityGroupRules[1:] // drops the 5432 ingress rule, keeps egress
			}), "data", "sg_dest_ingress"},
		{"turn the NACL's ingress allow into a deny",
			mutate("aws_network_acl.default", func(n *core.CanvasNode) {
				n.NACLRules[0].Action = "deny"
			}), "web", "nacl"},
		{"remove the private route table's only route (route table)",
			mutate("aws_route_table.private", func(n *core.CanvasNode) {
				n.Routes = nil
			}), "api", "route_selection"},
	}
	for _, c := range cases {
		got := flows(c.tpl)[c.journey]
		if got.Flows {
			t.Errorf("%s: journey %q must fail but still flows", c.name, c.journey)
			continue
		}
		if !strings.Contains(got.BlockedReason, c.step) {
			t.Errorf("%s: journey %q blocked, but not at %q — %s", c.name, c.journey, c.step, got.BlockedReason)
		}
	}
}

// PC-138/PC-139: missing data is NOT a verdict (I4). Take the working 3-tier design and
// remove, in turn, a subnet's route table and its NACL association: the trace step that
// needed it must read not_assessable — not deny, not allow — and say what is missing.
// And NACLs are stateless and directional: breaking only the EGRESS rule fails the
// journey too, so egress rules authored on the canvas are really evaluated, not assumed.
func TestThreeTier_MissingNetworkControlsAreNotAssessable_AndReturnPathIsEvaluated(t *testing.T) {
	reg := registry(t)
	traceOf := func(mutate func(*templates.Template)) []core.TraceStep {
		tpl, err := templates.Load("three-tier-vpc")
		if err != nil {
			t.Fatal(err)
		}
		mutate(&tpl)
		res, err := ingest.IngestCanvas(tpl.Canvas, reg, 1)
		if err != nil || res.IR == nil {
			t.Fatalf("ingest: %v / %+v", err, res.Insufficient)
		}
		return core.BuildTrace(res.IR, "aws_lb.web", "aws_eks_cluster.app", "", "tcp", 8080).Steps
	}
	step := func(steps []core.TraceStep, name string) (core.TraceStep, bool) {
		for _, s := range steps {
			if s.Step == name {
				return s, true
			}
		}
		return core.TraceStep{}, false
	}

	// Baseline: the route, both forward NACL legs (source egress, destination ingress) and
	// the SG step all run and allow. (The stateless return-path step only appears when the
	// return leg is the one that fails.)
	base := traceOf(func(*templates.Template) {})
	for _, name := range []string{"route_selection", "nacl_source_egress", "nacl_dest_ingress", "sg_dest_ingress"} {
		s, ok := step(base, name)
		if !ok || s.Decision != core.TraceAllow {
			t.Fatalf("baseline step %q: got %+v (present=%v), want allow", name, s, ok)
		}
	}

	// No route table for the destination's subnets.
	noRoute := traceOf(func(tpl *templates.Template) {
		for i := range tpl.Canvas.Nodes {
			if tpl.Canvas.Nodes[i].ID == "aws_route_table.private" {
				tpl.Canvas.Nodes[i].Routes = nil
			}
		}
	})
	if s, ok := step(noRoute, "route_selection"); !ok || s.Decision != core.TraceNotAssessable || !strings.Contains(s.Reason, "route table") {
		t.Errorf("no route table: route_selection must be not_assessable naming the missing route table, got %+v (present=%v)", s, ok)
	}

	// No NACL associated with the destination's subnets (PC-149): AWS associates such a
	// subnet with the VPC default NACL, which allows all traffic; with nothing declared
	// the engine applies that default, tagged assumed, and says so in the step reason.
	noNACL := traceOf(func(tpl *templates.Template) {
		kept := tpl.Canvas.Edges[:0]
		for _, e := range tpl.Canvas.Edges {
			if e.Type == "depends_on" && e.To == "aws_network_acl.default" {
				continue
			}
			kept = append(kept, e)
		}
		tpl.Canvas.Edges = kept
	})
	if s, ok := step(noNACL, "nacl_dest_ingress"); !ok || s.Decision != core.TraceAllow || s.Provenance.Kind != core.KindAssumed || !strings.Contains(s.Reason, "unmodified") {
		t.Errorf("no NACL association: nacl_dest_ingress must allow under the assumed default NACL, tagged assumed, stating the unmodified-default assumption; got %+v (present=%v)", s, ok)
	}

	// Only the EGRESS rule denies: stateless NACLs must fail the journey on the leg that rule governs.
	egressDeny := traceOf(func(tpl *templates.Template) {
		for i := range tpl.Canvas.Nodes {
			if tpl.Canvas.Nodes[i].ID == "aws_network_acl.default" {
				tpl.Canvas.Nodes[i].NACLRules[1].Action = "deny" // rule 100 egress
			}
		}
	})
	denied := false
	for _, s := range egressDeny {
		if strings.HasPrefix(s.Step, "nacl") && s.Decision == core.TraceDeny {
			denied = true
		}
	}
	if !denied {
		t.Errorf("an egress-only deny must be caught by a NACL step (stateless), got steps %+v", egressDeny)
	}
}
