// Package valuemutation is PC-158's value-level mutation sweep: for each golden bundle it rewrites ONE
// attribute at a time, across every resource and every nested block, to an expression the engine cannot
// read, then re-runs ingest and every assessment and compares with the baseline.
//
// The rule it enforces is I4 at the value level (NFR-3's resource-deletion sweep never tested it): an
// unreadable value may make a result not_assessable, or leave it unchanged (the value was never read),
// but it must never flip an assessed result to a different verdict, and never make a result vanish.
// "Nothing found" must never mean "never saw it". PC-157 (a jsonencode() policy dropped silently) is the
// bug class this exists to catch; TestSweep_NegativeControl_RecreatesPC157 proves it still would.
package valuemutation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

// observation is everything the engine reports about a bundle, reduced to a verdict per key.
type observation map[string]string

const notAssessable = "not_assessable"

func verdictOf(f core.Finding) string {
	if f.Outcome.State == core.AssessmentStateNotAssessable {
		return notAssessable
	}
	return fmt.Sprintf("%s:%v", f.Outcome.State, f.Outcome.Value)
}

func traceVerdict(tr core.Trace) string {
	if tr.Allowed {
		return "allow"
	}
	for _, s := range tr.Steps {
		switch s.Decision {
		case core.TraceNotAssessable:
			return notAssessable
		case core.TraceDeny:
			return "deny"
		}
	}
	return "deny"
}

// observe runs ingest and every assessment over dir. insufficient is true when the bundle no longer
// clears the minimum viable graph (an honest, structured not-assessable response, never a pass).
func observe(t *testing.T, dir string, workload core.Workload, blind bool) (obs observation, insufficient bool) {
	t.Helper()
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatal(err)
	}
	res, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("ingest %s: %v", dir, err)
	}
	if res.IR == nil {
		return nil, true
	}
	ir := res.IR
	if blind {
		blindToUnresolved(ir)
	}
	obs = observation{}
	for _, f := range core.BuildFindings(ir, workload) {
		obs["finding|"+f.ID] = verdictOf(f)
	}
	for _, set := range [][]core.ComplianceControlResult{
		core.BuildPCIDSS4Catalog(ir, workload), core.BuildSOC2Catalog(ir, workload), core.BuildCISAWSCatalog(ir, workload),
	} {
		for _, r := range set {
			obs["compliance|"+r.ControlID+"|"+r.NodeID] = string(r.Result.Status)
		}
	}
	// IAM: every role against every object store (the holder of a resource-based policy) and a few actions,
	// so an unreadable identity policy or bucket policy shows up as a changed decision.
	for _, role := range ir.Nodes {
		if role.Type != core.NodeTypeIdentity {
			continue
		}
		for _, store := range ir.Nodes {
			if store.Type != core.NodeTypeObjectStore {
				continue
			}
			for _, action := range []string{"s3:GetObject", "s3:PutObject", "sqs:SendMessage"} {
				res := core.EvaluateIAMRequest(ir, core.IAMRequest{PrincipalID: role.ID, Action: action, ResourceARN: "arn:aws:s3:::data/obj", ResourceID: store.ID,
					PrincipalIdentifier: "arn:aws:iam::123456789012:role/" + strings.TrimPrefix(role.ID, "aws_iam_role.")}, core.NewProvenance(core.KindDerived, "sweep"))
				v := string(res.Decision)
				if res.Decision == core.IAMDecisionNotAssessable {
					v = notAssessable
				}
				obs["iam|"+role.ID+"|"+store.ID+"|"+action] = v
			}
		}
	}
	for _, j := range workload.Journeys {
		for i := 0; i+1 < len(j.Path); i++ {
			from, to := j.Path[i], j.Path[i+1]
			port := j.PortForHop(to)
			var tr core.Trace
			switch {
			case from == core.JourneyInternetSentinel:
				tr = core.BuildTrace(ir, "", to, "0.0.0.0/0", j.Protocol, port)
			case to == core.JourneyInternetSentinel:
				tr = core.BuildOutboundTrace(ir, from, j.Protocol, port)
			default:
				tr = core.BuildTrace(ir, from, to, "", j.Protocol, port)
			}
			obs["trace|"+j.ID+"|"+from+">"+to] = traceVerdict(tr)
		}
	}
	return obs, false
}

// blindToUnresolved models an engine that ignores PC-158's markers: it strips every record of an unreadable
// input from the IR before assessing. It is the in-CI negative control: the sweep must FAIL against it,
// proving the sweep detects exactly the dependency PC-157 broke (an unreadable value dropped silently).
func blindToUnresolved(ir *core.IR) {
	for i := range ir.Nodes {
		delete(ir.Nodes[i].RawAttributes, core.UnresolvedInputsAttr)
		delete(ir.Nodes[i].RawAttributes, "unresolved_identity_policies")
	}
}

// violations lists every way mutated breaks the rule against base.
func violations(base, mutated observation) []string {
	var out []string
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b := base[k]
		m, ok := mutated[k]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("VANISHED  %s (was %s)", k, b))
		case m == b || m == notAssessable:
			// unchanged, or honestly not assessable: fine
		default:
			out = append(out, fmt.Sprintf("FLIPPED   %s: %s -> %s", k, b, m))
		}
	}
	return out
}

type target struct {
	file  string
	path  []int // block indices from the file's top level down to the block holding attr
	attr  string
	label string
}

// enumerate lists every attribute of every resource block (recursing into nested blocks) in a file.
func enumerate(file, src string) []target {
	f, diags := hclwrite.ParseConfig([]byte(src), file, hcl.InitialPos)
	if diags.HasErrors() {
		return nil
	}
	var out []target
	var walk func(body *hclwrite.Body, path []int, label string)
	walk = func(body *hclwrite.Body, path []int, label string) {
		names := make([]string, 0)
		for name := range body.Attributes() {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if name == "count" || name == "for_each" || name == "provider" {
				continue // already surfaced as unresolved constructs by ingest
			}
			out = append(out, target{file: file, path: append([]int(nil), path...), attr: name, label: label + "." + name})
		}
		for i, b := range body.Blocks() {
			walk(b.Body(), append(append([]int(nil), path...), i), label+"/"+b.Type())
		}
	}
	for i, b := range f.Body().Blocks() {
		if b.Type() != "resource" || len(b.Labels()) != 2 {
			continue
		}
		walk(b.Body(), []int{i}, b.Labels()[0]+"."+b.Labels()[1])
	}
	return out
}

// bodyAt follows path (block indices) from the top level to the nested body.
func bodyAt(f *hclwrite.File, path []int) *hclwrite.Body {
	body := f.Body()
	for _, idx := range path {
		body = body.Blocks()[idx].Body()
	}
	return body
}

// mutation kinds: an unknown function call, an unresolved reference, and malformed JSON text.
var kinds = map[string]string{
	"fn":      `unreadable_fn("x")`,
	"ref":     `var.unreadable_value`,
	"json":    `"{not json"`,
	"partial": ``, // the original expression with an unreadable part added: concat(<original>, var.unreadable_value)
}

func raw(s string) hclwrite.Tokens {
	return hclwrite.Tokens{{Type: hclsyntax.TokenIdent, Bytes: []byte(s)}}
}

func writeMutated(t *testing.T, srcDir, dstDir string, files map[string]string, tg target, kind, expr string) bool {
	t.Helper()
	for name, src := range files {
		out := src
		if name == tg.file {
			f, diags := hclwrite.ParseConfig([]byte(src), name, hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("parse %s: %v", name, diags)
			}
			body := bodyAt(f, tg.path)
			if kind == "partial" {
				orig := strings.TrimSpace(string(body.GetAttribute(tg.attr).Expr().BuildTokens(nil).Bytes()))
				if strings.Contains(orig, "\n") {
					return false // multi-line values (documents, objects) are covered by the other kinds
				}
				expr = "concat(" + orig + ", var.unreadable_value)"
			}
			body.SetAttributeRaw(tg.attr, raw(expr))
			out = string(f.Bytes())
		}
		if err := os.WriteFile(filepath.Join(dstDir, name), []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return true
}

func readBundle(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = string(b)
	}
	return files
}

// sweep mutates every attribute of bundle once per applicable kind and returns violations by mutation.
func sweep(t *testing.T, bundle string, workload core.Workload, blind bool, only func(target, string) bool) (found map[string][]string, total int) {
	t.Helper()
	files := readBundle(t, bundle)
	base, insufficient := observe(t, bundle, workload, blind)
	if insufficient {
		t.Fatalf("baseline %s is insufficient", bundle)
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	found = map[string][]string{}
	scratch := t.TempDir()
	for _, name := range names {
		for _, tg := range enumerate(name, files[name]) {
			for kind, expr := range kinds {
				if kind == "json" && !strings.Contains(tg.attr, "policy") {
					continue // malformed-JSON text only makes sense for a document-valued attribute
				}
				if only != nil && !only(tg, kind) {
					continue
				}
				if !writeMutated(t, bundle, scratch, files, tg, kind, expr) {
					continue
				}
				total++
				got, insufficient := observe(t, scratch, workload, blind)
				if insufficient {
					continue // below the minimum viable graph: a structured not-assessable response
				}
				if v := violations(base, got); len(v) > 0 {
					found[tg.label+" ["+kind+"]"] = v
				}
			}
		}
	}
	return found, total
}

func loadWorkload(t *testing.T) core.Workload {
	t.Helper()
	w, err := ingest.LoadWorkload(filepath.Join("..", "..", "golden", "workload.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestSweep_PrintFindings(t *testing.T) {
	if os.Getenv("SWEEP_REPORT") == "" {
		t.Skip("set SWEEP_REPORT=1 to print every violation (triage aid)")
	}
	w := loadWorkload(t)
	for _, b := range []string{"aws", "aws-broken"} {
		found, total := sweep(t, filepath.Join("..", "..", "golden", b), w, false, nil)
		keys := make([]string, 0, len(found))
		for k := range found {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("=== %s: %d mutations, %d with violations\n", b, total, len(keys))
		for _, k := range keys {
			fmt.Printf("%s\n", k)
			for _, v := range found[k] {
				fmt.Printf("      %s\n", v)
			}
		}
	}
}

// ---------------------------------------------------------------------------------------------------
// The assertions.

func goldenDir(b string) string { return filepath.Join("..", "..", "golden", b) }

// TestSweep_GoldenBundles_NoUnreadableValueReadsAsAVerdict is the sweep itself: across every attribute of
// every resource in both golden bundles, an unreadable value may make a result not_assessable or leave it
// unchanged, and must never flip it to a different verdict or make it vanish.
func TestSweep_GoldenBundles_NoUnreadableValueReadsAsAVerdict(t *testing.T) {
	w := loadWorkload(t)
	for _, b := range []string{"aws", "aws-broken"} {
		found, total := sweep(t, goldenDir(b), w, false, nil)
		if total < 500 {
			t.Errorf("%s: only %d mutations ran: the sweep is not covering the bundle", b, total)
		}
		for mut, vs := range found {
			t.Errorf("%s: mutating %s changes results other than to not_assessable:\n   %s", b, mut, strings.Join(vs, "\n   "))
		}
		t.Logf("%s: %d mutations, %d violations", b, total, len(found))
	}
}

// A sweep that compares mutated runs with a baseline cannot see a bug that blinds the BASELINE (that is what
// PC-157 was: the policy was never read, so making it unreadable changed nothing). So the seeded defects of
// the deliberately broken bundle must be detected as they stand.
func TestBaseline_SeededDefectsAreDetected(t *testing.T) {
	w := loadWorkload(t)
	obs, insufficient := observe(t, goldenDir("aws-broken"), w, false)
	if insufficient {
		t.Fatal("aws-broken is insufficient")
	}
	for key, want := range map[string]string{
		"finding|finding.compliance.iam-least-privilege-wildcard-admin.aws_iam_role.payments_app": "assessed:unsatisfied", // defect 7 (PC-157)
		"finding|finding.compliance.rds-storage-encryption.aws_db_instance.payments":              "assessed:unsatisfied",
		"finding|finding.compliance.nat-gateway-redundancy":                                       "assessed:unsatisfied",
		"compliance|pci_dss_4:7.2.2|aws_iam_role.payments_app":                                    "unsatisfied",
		"compliance|soc2:CC6.3|aws_iam_role.payments_app":                                         "unsatisfied",
	} {
		if got := obs[key]; got != want {
			t.Errorf("%s = %q, want %q: a seeded defect is not being detected", key, got, want)
		}
	}
}

// dependency says: making this attribute unreadable must turn these results not_assessable (not merely leave
// them alone). It is the declared-read side of the sweep: unchanged is acceptable for an attribute nothing
// reads, but not for one the engine is known to depend on.
type dependency struct {
	bundle, attr string
	keys         []string
}

var declaredDependencies = []dependency{
	{"aws-broken", "aws_iam_policy.payments_app.policy", []string{
		"finding|finding.compliance.iam-least-privilege-wildcard-admin.aws_iam_role.payments_app",
		"compliance|pci_dss_4:7.2.2|aws_iam_role.payments_app", "compliance|soc2:CC6.3|aws_iam_role.payments_app"}},
	// PC-159: golden/aws's own policy references in-bundle ARNs; making it unreadable must withdraw the verdict.
	{"aws", "aws_iam_policy.payments_app.policy", []string{
		"finding|finding.compliance.iam-least-privilege-wildcard-admin.aws_iam_role.payments_app",
		"compliance|pci_dss_4:7.2.2|aws_iam_role.payments_app", "compliance|soc2:CC6.3|aws_iam_role.payments_app"}},
	{"aws", "aws_db_instance.payments.storage_encrypted", []string{
		"finding|finding.compliance.rds-storage-encryption.aws_db_instance.payments",
		"compliance|pci_dss_4:3.5.1.2|aws_db_instance.payments", "compliance|cis_aws.storage_encryption|aws_db_instance.payments"}},
	{"aws", "aws_db_instance.payments.multi_az", []string{
		"finding|finding.compliance.rds-rpo-feasibility.aws_db_instance.payments", "compliance|soc2:A1.2|aws_db_instance.payments"}},
	{"aws", "aws_security_group.database/ingress.to_port", []string{"trace|checkout|aws_eks_cluster.payments>aws_db_instance.payments"}},
	{"aws", "aws_security_group.workload/egress.cidr_blocks", []string{"trace|payment-rail|aws_eks_cluster.payments>internet"}},
	{"aws", "aws_route_table.private_a/route.nat_gateway_id", []string{"trace|payment-rail|aws_eks_cluster.payments>internet"}},
	{"aws", "aws_nat_gateway.nat_a.subnet_id", []string{"finding|finding.compliance.nat-gateway-redundancy", "compliance|cis_aws.nat_gateway_redundancy|"}},
	{"aws", "aws_subnet.public_a.tags", []string{"finding|finding.routing.public-subnet-label.aws_subnet.public_a"}},
	{"aws", "aws_lb.payments.internal", []string{"compliance|pci_dss_4:6.4.2|aws_lb.payments"}},
	{"aws", "aws_db_instance.payments.vpc_security_group_ids", []string{"compliance|pci_dss_4:1.3.1|aws_db_instance.payments", "compliance|pci_dss_4:1.3.2|aws_db_instance.payments"}},
	{"aws", "aws_db_instance.payments.db_subnet_group_name", []string{"finding|finding.zone-kill.data-a"}},
}

func TestSweep_DeclaredDependenciesBecomeNotAssessable(t *testing.T) {
	w := loadWorkload(t)
	for _, d := range declaredDependencies {
		bundle := goldenDir(d.bundle)
		base, _ := observe(t, bundle, w, false)
		files := readBundle(t, bundle)
		hit := false
		for name := range files {
			for _, tg := range enumerate(name, files[name]) {
				if tg.label != d.attr {
					continue
				}
				hit = true
				for _, kind := range []string{"fn", "ref", "partial"} {
					scratch := t.TempDir()
					if !writeMutated(t, bundle, scratch, files, tg, kind, kinds[kind]) {
						continue
					}
					got, insufficient := observe(t, scratch, w, false)
					if insufficient {
						t.Fatalf("%s [%s]: bundle became insufficient", d.attr, kind)
					}
					for _, k := range d.keys {
						if _, ok := base[k]; !ok {
							t.Errorf("%s: declared dependency key %q is not in the baseline: fix the table", d.attr, k)
							continue
						}
						if got[k] != notAssessable {
							t.Errorf("%s [%s]: %s = %q, want not_assessable (baseline %q): the engine reads this attribute but an unreadable value does not withdraw the verdict", d.attr, kind, k, got[k], base[k])
						}
					}
				}
			}
		}
		if !hit {
			t.Errorf("%s: no such attribute in %s: fix the table", d.attr, d.bundle)
		}
	}
}

// The in-CI negative control (PC-158 criterion: "reintroduce the PC-157 drop, confirm the sweep fails"). An
// engine that ignores the unreadable-input records silently reads an unreadable value as a verdict; the
// sweep must catch it, on the very attribute PC-157 was about and on others.
func TestSweep_NegativeControl_EngineBlindToUnreadableValues(t *testing.T) {
	w := loadWorkload(t)
	policy := func(tg target, kind string) bool { return tg.label == "aws_iam_policy.payments_app.policy" }
	found, total := sweep(t, goldenDir("aws-broken"), w, true, policy)
	if total == 0 || len(found) == 0 {
		t.Fatalf("a blind engine passed the sweep on the PC-157 attribute (%d mutations, %d violations): the sweep cannot catch the bug class", total, len(found))
	}
	joined := fmt.Sprint(found)
	if !strings.Contains(joined, "iam-least-privilege-wildcard-admin.aws_iam_role.payments_app") {
		t.Errorf("blind engine: the wildcard-admin finding did not flip: %s", joined)
	}

	// And it catches the other mechanisms too (security-group rule, placement, tags).
	for _, label := range []string{"aws_security_group.database/ingress.to_port", "aws_nat_gateway.nat_a.subnet_id", "aws_subnet.public_a.tags"} {
		label := label
		found, _ := sweep(t, goldenDir("aws"), w, true, func(tg target, kind string) bool { return tg.label == label })
		if len(found) == 0 {
			t.Errorf("blind engine passed the sweep on %s", label)
		}
	}
}

func TestComparator(t *testing.T) {
	base := observation{"a": "assessed:satisfied", "b": "assessed:unsatisfied", "c": notAssessable, "d": "allow"}
	cases := map[string]struct {
		mutated observation
		want    int
	}{
		"unchanged":                          {observation{"a": "assessed:satisfied", "b": "assessed:unsatisfied", "c": notAssessable, "d": "allow"}, 0},
		"everything honestly not assessable": {observation{"a": notAssessable, "b": notAssessable, "c": notAssessable, "d": notAssessable}, 0},
		"a verdict flips":                    {observation{"a": "assessed:unsatisfied", "b": "assessed:unsatisfied", "c": notAssessable, "d": "allow"}, 1},
		"unsatisfied reads as satisfied":     {observation{"a": "assessed:satisfied", "b": "assessed:satisfied", "c": notAssessable, "d": "allow"}, 1},
		"a result vanishes":                  {observation{"a": "assessed:satisfied", "c": notAssessable, "d": "allow"}, 1},
		"not assessable becomes a pass":      {observation{"a": "assessed:satisfied", "b": "assessed:unsatisfied", "c": "assessed:satisfied", "d": "allow"}, 1},
	}
	for name, c := range cases {
		if got := len(violations(base, c.mutated)); got != c.want {
			t.Errorf("%s: %d violations, want %d", name, got, c.want)
		}
	}
}

// kitchenWorkload exercises the kitchen-sink bundle: an inbound journey through the load balancer to the
// instance and on to the database, and an outbound journey from the public-subnet worker through the
// internet gateway (which needs a public address).
func kitchenWorkload() core.Workload {
	return core.Workload{Journeys: []core.DeclaredJourney{
		{ID: "inbound", Name: "inbound", Path: []string{"internet", "aws_lb.front", "aws_instance.web", "aws_db_instance.d"},
			Protocol: "tcp", Port: 443, HopPorts: map[string]int{"aws_lb.front": 443, "aws_instance.web": 8080, "aws_db_instance.d": 5432}, Criticality: "tier1"},
		{ID: "outbound-igw", Name: "outbound via IGW", Path: []string{"aws_instance.worker", "internet"}, Protocol: "tcp", Port: 443, Criticality: "tier2"},
		{ID: "outbound-nat", Name: "outbound via NAT", Path: []string{"aws_instance.web", "internet"}, Protocol: "tcp", Port: 443, Criticality: "tier2"},
	}}
}

func TestSweep_KitchenSink_NoUnreadableValueReadsAsAVerdict(t *testing.T) {
	bundle := filepath.Join("testdata", "kitchen")
	w := kitchenWorkload()
	found, total := sweep(t, bundle, w, false, nil)
	if total < 300 {
		t.Errorf("only %d mutations ran on the kitchen-sink bundle", total)
	}
	for mut, vs := range found {
		t.Errorf("kitchen: mutating %s changes results other than to not_assessable:\n   %s", mut, strings.Join(vs, "\n   "))
	}
	t.Logf("kitchen: %d mutations, %d violations", total, len(found))
}

// The kitchen-sink baseline must actually exercise the engine: a mix of allow, deny and not_assessable, and every
// construct reaching a result (otherwise a clean sweep would prove nothing).
func TestKitchenSink_BaselineExercisesTheEngine(t *testing.T) {
	obs, insufficient := observe(t, filepath.Join("testdata", "kitchen"), kitchenWorkload(), false)
	if insufficient {
		t.Fatal("the kitchen-sink bundle is insufficient")
	}
	counts := map[string]int{}
	for k, v := range obs {
		if strings.HasPrefix(k, "trace|") {
			counts[v]++
		}
	}
	t.Logf("traces: %v; total observations: %d", counts, len(obs))
	if counts["allow"] == 0 {
		t.Error("no journey hop is allowed in the baseline: the sweep would be testing nothing")
	}
	if len(obs) < 60 {
		t.Errorf("only %d observations: the bundle is not reaching the engine", len(obs))
	}
}

// And the blind-engine control on the kitchen bundle.
func TestSweep_KitchenSink_NegativeControl_BlindEngineIsCaught(t *testing.T) {
	found, total := sweep(t, filepath.Join("testdata", "kitchen"), kitchenWorkload(), true, nil)
	if total == 0 || len(found) < 10 {
		t.Fatalf("a blind engine passed the kitchen-sink sweep (%d mutations, %d violations): the sweep is not sensitive enough", total, len(found))
	}
}
