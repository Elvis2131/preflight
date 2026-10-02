package rung3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"preflight/core"
)

// fakeCloud stands in for terraform, the aws CLI and the ALB. Nothing here touches a real account.
type fakeCloud struct {
	mu          sync.Mutex
	account     string
	budgets     bool
	budgetAlert bool
	applyErr    error
	fisErr      error
	destroyErrs int // fail this many destroy attempts first
	leftover    []string
	calls       []string
	faultAt     atomic.Int64 // unix nanos, 0 = not started
	albURL      string
}

func (f *fakeCloud) exec(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	f.mu.Unlock()
	joined := strings.Join(args, " ")
	switch {
	case name == "aws" && strings.HasPrefix(joined, "sts get-caller-identity"):
		return []byte(fmt.Sprintf(`{"Account":%q}`, f.account)), nil
	case name == "aws" && strings.HasPrefix(joined, "budgets describe-budgets"):
		if !f.budgets {
			return []byte(`{"Budgets":[]}`), nil
		}
		return []byte(`{"Budgets":[{"BudgetName":"b"}]}`), nil
	case name == "aws" && strings.HasPrefix(joined, "budgets describe-notifications"):
		if !f.budgetAlert {
			return []byte(`{"Notifications":[]}`), nil
		}
		return []byte(`{"Notifications":[{"Threshold":50}]}`), nil
	case name == "terraform" && strings.Contains(joined, " apply "):
		return nil, f.applyErr
	case name == "terraform" && strings.Contains(joined, " destroy "):
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.destroyErrs > 0 {
			f.destroyErrs--
			return nil, errors.New("destroy failed")
		}
		return nil, nil
	case name == "terraform" && strings.Contains(joined, " output "):
		return []byte(fmt.Sprintf(`{"alb_dns_name":{"value":%q},"target_group_arn":{"value":"arn:tg"},"experiment_template_id":{"value":"EXT1"},"instance_a_id":{"value":"i-a"},"instance_b_id":{"value":"i-b"}}`, f.albURL)), nil
	case name == "terraform" && strings.Contains(joined, " state list"):
		return nil, nil
	case name == "terraform":
		return nil, nil
	case name == "aws" && strings.HasPrefix(joined, "elbv2 describe-target-health"):
		a := `{"State":"healthy"}`
		if at := f.faultAt.Load(); at != 0 && time.Since(time.Unix(0, at)) > 300*time.Millisecond {
			a = `{"State":"unused","Reason":"Target.InvalidState"}`
		}
		return []byte(fmt.Sprintf(`{"TargetHealthDescriptions":[{"Target":{"Id":"i-a"},"TargetHealth":%s},{"Target":{"Id":"i-b"},"TargetHealth":{"State":"healthy"}}]}`, a)), nil
	case name == "aws" && strings.HasPrefix(joined, "fis start-experiment"):
		if f.fisErr != nil {
			return nil, f.fisErr
		}
		f.faultAt.Store(time.Now().UnixNano())
		return []byte(`{"experiment":{"id":"EXP1"}}`), nil
	case name == "aws" && strings.HasPrefix(joined, "fis get-experiment"):
		return []byte(`{"experiment":{"state":{"status":"completed","reason":"done"}}}`), nil
	case name == "aws" && strings.HasPrefix(joined, "resourcegroupstaggingapi"):
		var parts []string
		for _, l := range f.leftover {
			parts = append(parts, fmt.Sprintf(`{"ResourceARN":%q}`, l))
		}
		return []byte(`{"ResourceTagMappingList":[` + strings.Join(parts, ",") + `]}`), nil
	}
	return nil, fmt.Errorf("fake: unexpected command %s %s", name, joined)
}

func (f *fakeCloud) sawCall(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

// newFake starts a pretend ALB: it alternates web-a / web-b; for 400 ms after the fault it also
// errors on web-a's turn (the ALB has not noticed yet), then serves web-b only.
func newFake(t *testing.T) *fakeCloud {
	t.Helper()
	f := &fakeCloud{account: "111", budgets: true, budgetAlert: true}
	var n atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn := n.Add(1)
		at := f.faultAt.Load()
		since := time.Since(time.Unix(0, at))
		faulted := at != 0
		switch {
		case faulted && since > 400*time.Millisecond:
			fmt.Fprint(w, "web-b")
		case faulted && turn%2 == 0:
			http.Error(w, "bad gateway", http.StatusBadGateway)
		case turn%2 == 0:
			fmt.Fprint(w, "web-a")
		default:
			fmt.Fprint(w, "web-b")
		}
	}))
	t.Cleanup(srv.Close)
	f.albURL = strings.TrimPrefix(srv.URL, "http://")
	return f
}

func testConfig(t *testing.T, f *fakeCloud) Config {
	t.Helper()
	pred := filepath.Join(t.TempDir(), "predictions.json")
	body := `{"statements":[{"id":"P1","claim":"survivor serves"},{"id":"P2","claim":"verdict"},{"id":"P3","claim":"window"},{"id":"P4","claim":"unused"}]}`
	if err := os.WriteFile(pred, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return Config{
		Account: "111", RunID: "t1", TerraformDir: "tf", PredictionsPath: pred, Exec: f.exec,
		BaselineFor: 300 * time.Millisecond, FaultWindow: 1500 * time.Millisecond,
		ProbeEvery: 10 * time.Millisecond, HealthEvery: 50 * time.Millisecond,
		StatePoll: 100 * time.Millisecond, WaitPoll: 10 * time.Millisecond, DestroyPoll: 5 * time.Millisecond,
		HealthyTimeout: 2 * time.Second,
	}
}

func TestPreconditions(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*fakeCloud, *Config)
		want string
	}{
		{"no account named", func(_ *fakeCloud, c *Config) { c.Account = "" }, "-account is required"},
		{"wrong account", func(f *fakeCloud, _ *Config) { f.account = "999" }, "not the named sandbox"},
		{"no budget", func(f *fakeCloud, _ *Config) { f.budgets = false }, "no budget with an alert"},
		{"budget without an alert", func(f *fakeCloud, _ *Config) { f.budgetAlert = false }, "no budget with an alert"},
	}
	for _, tc := range cases {
		f := newFake(t)
		cfg := testConfig(t, f)
		tc.mut(f, &cfg)
		err := Preconditions(context.Background(), cfg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	f := newFake(t)
	if err := Preconditions(context.Background(), testConfig(t, f)); err != nil {
		t.Errorf("a valid account with a budget alert must pass: %v", err)
	}
}

func TestRun_RefusedPreconditionsCreateNothing(t *testing.T) {
	f := newFake(t)
	f.budgets = false
	if _, err := Run(context.Background(), testConfig(t, f)); err == nil {
		t.Fatal("expected a refusal")
	}
	if f.sawCall(" apply ") || f.sawCall(" init ") || f.sawCall("fis start-experiment") {
		t.Errorf("nothing may be applied or started when a precondition fails; calls: %v", f.calls)
	}
}

func TestRun_HappyPath_ObservesAndDestroys(t *testing.T) {
	f := newFake(t)
	res, err := Run(context.Background(), testConfig(t, f))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Provenance.Kind != core.KindObserved || *res.Provenance.Rung != core.Rung3RealCloud || *res.Provenance.EvidenceTier != core.EvidenceTierResilience {
		t.Errorf("a real run must be observed / rung 3 / resilience, got %+v", res.Provenance)
	}
	if err := res.Provenance.Validate(); err != nil {
		t.Errorf("provenance invalid: %v", err)
	}
	if !res.Cleanup.Clean || !f.sawCall(" destroy ") {
		t.Errorf("destroy must run and verify clean: %+v", res.Cleanup)
	}
	if res.Observed.BaselineFailures != 0 || len(res.Observed.BaselineServedBy) != 2 {
		t.Errorf("baseline should be clean and served by both: %+v", res.Observed)
	}
	if res.Observed.FailuresAfterFault == 0 || res.Observed.FirstFailureSeconds == nil {
		t.Errorf("the fake ALB fails for a while after the fault: %+v", res.Observed)
	}
	if got := res.Observed.ServedByAfterLastFail; len(got) != 1 || got[0] != "web-b" {
		t.Errorf("after the window only web-b serves, got %v", got)
	}
	if res.Observed.WebAFinalState != "unused" || res.Observed.WebAFinalReason != "Target.InvalidState" {
		t.Errorf("target health should be read through: %+v", res.Observed)
	}
	verdicts := map[string]string{}
	for _, c := range res.Comparison {
		verdicts[c.ID] = c.Verdict
	}
	if verdicts["P1"] != "confirmed" || verdicts["P4"] != "confirmed" || verdicts["P2"] != "not_observed" {
		t.Errorf("verdicts = %v", verdicts)
	}
}

func TestRun_DestroysEvenWhenTheRunFails(t *testing.T) {
	for name, mut := range map[string]func(*fakeCloud){
		"apply fails (a partial apply may exist)": func(f *fakeCloud) { f.applyErr = errors.New("apply boom") },
		"the fault cannot be started":             func(f *fakeCloud) { f.fisErr = errors.New("fis boom") },
	} {
		f := newFake(t)
		mut(f)
		res, err := Run(context.Background(), testConfig(t, f))
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if !f.sawCall(" destroy ") {
			t.Errorf("%s: destroy MUST still run", name)
		}
		if !res.Cleanup.Clean {
			t.Errorf("%s: cleanup should be verified clean: %+v", name, res.Cleanup)
		}
	}
}

func TestRun_DestroyIsRetriedAndLeftoversAreReported(t *testing.T) {
	f := newFake(t)
	f.destroyErrs = 2
	res, err := Run(context.Background(), testConfig(t, f))
	if err != nil || !res.Cleanup.Clean {
		t.Fatalf("destroy should succeed on the 3rd attempt: err=%v cleanup=%+v", err, res.Cleanup)
	}

	f = newFake(t)
	f.leftover = []string{"arn:aws:elasticloadbalancing:us-east-1:111:loadbalancer/app/x"}
	res, err = Run(context.Background(), testConfig(t, f))
	if err == nil || !strings.Contains(err.Error(), "CLEANUP INCOMPLETE") || res.Cleanup.Clean {
		t.Fatalf("a tagged leftover must fail the run loudly: err=%v cleanup=%+v", err, res.Cleanup)
	}
	if len(res.Cleanup.TaggedLeftover) != 1 {
		t.Errorf("the leftover must be named: %+v", res.Cleanup)
	}
}

func TestRun_CancelledContextStillDestroys(t *testing.T) {
	f := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cfg := testConfig(t, f)
	cfg.BaselineFor = 5 * time.Second
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	_, err := Run(ctx, cfg)
	if err == nil {
		t.Fatal("a cancelled run should report it")
	}
	if !f.sawCall(" destroy ") {
		t.Error("a cancelled run context must not stop the teardown")
	}
}

func TestAnalyse_NumbersFromATimeline(t *testing.T) {
	pr := func(at int64, status int, body string) Probe { return Probe{AtMs: at, Status: status, Body: body} }
	probes := []Probe{
		pr(-2000, 200, "web-a"), pr(-1000, 200, "web-b"),
		pr(1000, 200, "web-b"), pr(2000, 502, ""), pr(3000, 200, "web-b"), pr(4000, 502, ""), pr(5000, 200, "web-b"), pr(6000, 200, "web-b"),
	}
	h := []HealthSample{
		{AtMs: 0, Targets: map[string]Target{"web-a": {State: "healthy"}}},
		{AtMs: 6000, Targets: map[string]Target{"web-a": {State: "unhealthy", Reason: "Target.FailedHealthChecks"}}},
		{AtMs: 9000, Targets: map[string]Target{"web-a": {State: "unused", Reason: "Target.InvalidState"}}},
	}
	o := Analyse(probes, h, "completed")
	if *o.FirstFailureSeconds != 2 || *o.LastFailureSeconds != 4 || *o.FailureWindowSeconds != 2 {
		t.Errorf("window wrong: %+v", o)
	}
	if got := *o.FailureShareInWindow; got < 0.66 || got > 0.67 { // 2 failures of 3 probes in [2000,4000]
		t.Errorf("share = %v", got)
	}
	if *o.WebAFirstNotHealthySec != 6 || o.WebAFinalState != "unused" {
		t.Errorf("health reading wrong: %+v", o)
	}
	if o.BaselineProbes != 2 || o.ProbesAfterFault != 6 || o.FailuresAfterFault != 2 {
		t.Errorf("counts wrong: %+v", o)
	}
}

func TestCompare_RefutesWhenTheRunDisagrees(t *testing.T) {
	pred := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(pred, []byte(`{"statements":[{"id":"P1","claim":"c"},{"id":"P3","claim":"c"},{"id":"P4","claim":"c"}]}`), 0o644)
	// nothing ever failed, web-a stayed "healthy": P3 (some requests fail) and P4 must be refuted.
	var probes []Probe
	for i := int64(0); i < 100; i++ {
		probes = append(probes, Probe{AtMs: i * 500, Status: 200, Body: "web-b"})
	}
	o := Analyse(probes, []HealthSample{{AtMs: 1000, Targets: map[string]Target{"web-a": {State: "healthy"}}}}, "completed")
	cmp, err := Compare(pred, probes, o)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cmp {
		got[c.ID] = c.Verdict
	}
	if got["P1"] != "confirmed" || got["P3"] != "refuted" || got["P4"] != "refuted" {
		t.Errorf("a misprediction must be recorded as refuted, got %v", got)
	}
	// too little data cannot confirm P1
	cmp, _ = Compare(pred, probes[:5], Analyse(probes[:5], nil, ""))
	for _, c := range cmp {
		if c.ID == "P1" && c.Verdict != "refuted" {
			t.Errorf("5 probes cannot confirm survival, got %s", c.Verdict)
		}
	}
}

func TestProbeFailureIsNot200(t *testing.T) {
	if !(Probe{Status: 0}).Failed() || !(Probe{Status: 502}).Failed() || (Probe{Status: 200}).Failed() {
		t.Error("only a 200 is a success")
	}
}

// I5: a Rung 1 or 2 result can never carry the resilience tier, whatever the producer says.
func TestObservedProvenance_CannotBeLaunderedFromAnotherRung(t *testing.T) {
	for _, rung := range []core.Rung{core.Rung1TopologyReplica, core.Rung2Emulated} {
		p := core.NewProvenance(core.KindObserved, "x").WithObserved(core.EvidenceTierResilience, rung)
		if err := p.Validate(); err == nil {
			t.Errorf("rung %d with the resilience tier must be rejected", rung)
		}
	}
	if err := observedProvenance("x").Validate(); err != nil {
		t.Errorf("this package's own provenance must be valid: %v", err)
	}
}

func TestResultJSONRoundTrips(t *testing.T) {
	f := newFake(t)
	res, err := Run(context.Background(), testConfig(t, f))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var back Result
	if err := json.Unmarshal(b, &back); err != nil || back.Provenance.Kind != core.KindObserved {
		t.Fatalf("round trip failed: %v", err)
	}
}

// observedProducers lists non-test Go files under root that build an observed provenance, other than
// the allowed ones. PC-25: "double check the observed-result path is wired to nothing else in the
// codebase that could relabel a Rung 1/2 result as observed by mistake."
func observedProducers(root string, allowed ...string) ([]string, error) {
	var bad []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".terraform", "worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, a := range allowed {
			if strings.HasPrefix(rel, a) {
				return nil
			}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if s := string(b); strings.Contains(s, "WithObserved(") || strings.Contains(s, "KindObserved") {
			bad = append(bad, rel)
		}
		return nil
	})
	return bad, err
}

func TestObservedIsOnlyProducedHere(t *testing.T) {
	root, _ := filepath.Abs("../../../..")
	bad, err := observedProducers(root, "core/provenance.go", "cmd/runnerd/internal/rung3/")
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Errorf("only the Rung 3 runner may produce an observed value; also found in: %v", bad)
	}

	// negative control: the scan really does catch a second producer.
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "validate"), 0o755)
	os.WriteFile(filepath.Join(tmp, "validate", "x.go"), []byte("package v\nvar _ = p.WithObserved(a, b)\n"), 0o644)
	got, _ := observedProducers(tmp)
	if len(got) != 1 {
		t.Errorf("the scan must flag a second producer, got %v", got)
	}
}
