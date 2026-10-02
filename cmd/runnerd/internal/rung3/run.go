package rung3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// Exec runs one external command and returns its stdout. The runner never builds an environment for
// it: terraform and the aws CLI resolve credentials from whatever this process inherited, so the
// runner itself never holds, reads or logs a key.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// OSExec is the real Exec.
func OSExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 800 {
			msg = msg[:800] + "…"
		}
		return out, fmt.Errorf("%s %s: %w: %s", name, firstArgs(args), err, msg)
	}
	return out, nil
}

func firstArgs(a []string) string {
	if len(a) > 2 {
		a = a[:2]
	}
	return strings.Join(a, " ")
}

// Config is everything one run needs.
type Config struct {
	Account         string // must equal the account the credentials resolve to
	Region          string
	TerraformDir    string
	PredictionsPath string
	RunID           string

	Exec Exec
	HTTP *http.Client
	Log  func(format string, args ...any)

	BaselineFor    time.Duration // clients only, before the fault
	FaultWindow    time.Duration // how long to watch after the fault starts
	ProbeEvery     time.Duration
	HealthEvery    time.Duration
	HealthyTimeout time.Duration
	StatePoll      time.Duration // how often the experiment state is read
	WaitPoll       time.Duration // how often target health is read while waiting for healthy
	DestroyPoll    time.Duration // pause between tagging-API leftover checks
}

func (c *Config) defaults() {
	if c.Exec == nil {
		c.Exec = OSExec
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	}
	if c.Log == nil {
		c.Log = func(string, ...any) {}
	}
	if c.Region == "" {
		c.Region = "eu-north-1"
	}
	if c.BaselineFor == 0 {
		c.BaselineFor = 20 * time.Second
	}
	if c.FaultWindow == 0 {
		c.FaultWindow = 150 * time.Second
	}
	if c.ProbeEvery == 0 {
		c.ProbeEvery = 200 * time.Millisecond
	}
	if c.HealthEvery == 0 {
		c.HealthEvery = 2 * time.Second
	}
	if c.StatePoll == 0 {
		c.StatePoll = 5 * time.Second
	}
	if c.WaitPoll == 0 {
		c.WaitPoll = 10 * time.Second
	}
	if c.DestroyPoll == 0 {
		c.DestroyPoll = 10 * time.Second
	}
	if c.HealthyTimeout == 0 {
		c.HealthyTimeout = 10 * time.Minute
	}
}

// Preconditions refuses to start unless the account is the one the operator named and a budget with
// at least one alert exists in it (PC-25: "before this runs, not after").
func Preconditions(ctx context.Context, c Config) error {
	c.defaults()
	if c.Account == "" {
		return errors.New("rung3: -account is required: name the sandbox account explicitly")
	}
	out, err := c.Exec(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return fmt.Errorf("rung3: cannot resolve AWS credentials: %w", err)
	}
	var id struct{ Account string }
	if err := json.Unmarshal(out, &id); err != nil {
		return err
	}
	if id.Account != c.Account {
		return fmt.Errorf("rung3: credentials resolve to account %s, not the named sandbox %s: refusing to run", id.Account, c.Account)
	}
	out, err = c.Exec(ctx, "aws", "budgets", "describe-budgets", "--account-id", c.Account, "--output", "json")
	if err != nil {
		return fmt.Errorf("rung3: cannot read budgets: %w", err)
	}
	var bs struct{ Budgets []struct{ BudgetName string } }
	if err := json.Unmarshal(out, &bs); err != nil {
		return err
	}
	for _, b := range bs.Budgets {
		n, err := c.Exec(ctx, "aws", "budgets", "describe-notifications-for-budget", "--account-id", c.Account, "--budget-name", b.BudgetName, "--output", "json")
		if err != nil {
			return fmt.Errorf("rung3: cannot read budget alerts: %w", err)
		}
		var ns struct{ Notifications []json.RawMessage }
		if err := json.Unmarshal(n, &ns); err == nil && len(ns.Notifications) > 0 {
			return nil
		}
	}
	return errors.New("rung3: no budget with an alert exists in the account: create the budget alarm first (PC-25 precondition)")
}

// Run applies, injects the fault, observes, and ALWAYS destroys. The returned Result is meaningful
// even when err is non-nil (it records how far the run got and what cleanup found).
func Run(ctx context.Context, c Config) (res Result, err error) {
	c.defaults()
	if err = Preconditions(ctx, c); err != nil {
		return res, err
	}
	res = Result{
		Experiment: "stop one of two ALB targets with EC2 StopInstances",
		RunID:      c.RunID, Account: c.Account, Region: c.Region, StartedAt: time.Now().UTC(),
		Probes: []Probe{}, Health: []HealthSample{}, Comparison: []Comparison{},
	}
	tf := func(cx context.Context, args ...string) ([]byte, error) {
		return c.Exec(cx, "terraform", append([]string{"-chdir=" + c.TerraformDir}, args...)...)
	}

	attempted := false
	defer func() {
		if !attempted {
			res.FinishedAt = time.Now().UTC()
			return
		}
		// A cancelled run context must not stop the teardown: it gets its own.
		dctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
		defer cancel()
		res.Cleanup = destroyAndVerify(dctx, c, tf)
		res.FinishedAt = time.Now().UTC()
		if !res.Cleanup.Clean {
			err = errors.Join(err, fmt.Errorf("rung3: CLEANUP INCOMPLETE, resources may still be running and billing in %s: %s", c.Account, res.Cleanup.Note))
		}
	}()

	c.Log("terraform init")
	if _, err = tf(ctx, "init", "-input=false", "-no-color"); err != nil {
		return res, err
	}
	attempted = true // from here on a partial apply may have created something
	c.Log("terraform apply (run %s)", c.RunID)
	if _, err = tf(ctx, "apply", "-auto-approve", "-input=false", "-no-color", "-var", "run_id="+c.RunID, "-var", "region="+c.Region); err != nil {
		return res, err
	}
	outs, err := terraformOutputs(ctx, tf)
	if err != nil {
		return res, err
	}
	names := map[string]string{outs["instance_a_id"]: "web-a", outs["instance_b_id"]: "web-b"}
	health := func(cx context.Context) (map[string]Target, error) {
		return describeHealth(cx, c, outs["target_group_arn"], names)
	}

	c.Log("waiting for both targets to be healthy")
	if err = waitHealthy(ctx, c, health); err != nil {
		return res, err
	}

	url := "http://" + outs["alb_dns_name"] + "/"
	var mu sync.Mutex
	var raw []rawProbe
	pctx, stopProbes := context.WithCancel(ctx)
	probesDone := make(chan struct{})
	go func() {
		defer close(probesDone)
		probeLoop(pctx, c, url, func(p rawProbe) { mu.Lock(); raw = append(raw, p); mu.Unlock() })
	}()
	defer func() { stopProbes(); <-probesDone }()

	c.Log("baseline: %s of clients only", c.BaselineFor)
	if err = sleep(ctx, c.BaselineFor); err != nil {
		return res, err
	}

	var samples []rawHealth
	hctx, stopHealth := context.WithCancel(ctx)
	healthDone := make(chan struct{})
	faultStart := time.Now()
	go func() {
		defer close(healthDone)
		for {
			if t, herr := health(hctx); herr == nil {
				samples = append(samples, rawHealth{at: time.Now(), t: t})
			}
			select {
			case <-hctx.Done():
				return
			case <-time.After(c.HealthEvery):
			}
		}
	}()

	c.Log("stopping instance %s (EC2 StopInstances)", outs["instance_a_id"])
	err = stopInstance(ctx, c, outs["instance_a_id"])
	if err != nil {
		stopHealth()
		<-healthDone
		return res, err
	}
	state := "unknown"
	deadline := faultStart.Add(c.FaultWindow)
	for time.Now().Before(deadline) {
		if serr := sleep(ctx, c.StatePoll); serr != nil {
			err = serr
			break
		}
		if s, gerr := instanceState(ctx, c, outs["instance_a_id"]); gerr == nil {
			state = s
		}
	}
	stopHealth()
	<-healthDone
	stopProbes()
	<-probesDone

	mu.Lock()
	res.Probes = relativeProbes(raw, faultStart)
	mu.Unlock()
	res.Health = relativeHealth(samples, faultStart)
	res.Provenance = observedProvenance("ec2-stop-instances")
	res.Observed = Analyse(res.Probes, res.Health, state)
	if cmp, cerr := Compare(c.PredictionsPath, res.Probes, res.Observed); cerr == nil {
		res.Comparison = cmp
	} else {
		err = errors.Join(err, cerr)
	}
	return res, err
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func terraformOutputs(ctx context.Context, tf func(context.Context, ...string) ([]byte, error)) (map[string]string, error) {
	b, err := tf(ctx, "output", "-json", "-no-color")
	if err != nil {
		return nil, err
	}
	var raw map[string]struct{ Value any }
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range raw {
		out[k] = fmt.Sprint(v.Value)
	}
	for _, need := range []string{"alb_dns_name", "target_group_arn", "instance_a_id", "instance_b_id"} {
		if out[need] == "" {
			return nil, fmt.Errorf("rung3: terraform output %q is missing", need)
		}
	}
	return out, nil
}

func describeHealth(ctx context.Context, c Config, tgArn string, names map[string]string) (map[string]Target, error) {
	b, err := c.Exec(ctx, "aws", "elbv2", "describe-target-health", "--target-group-arn", tgArn, "--region", c.Region, "--output", "json")
	if err != nil {
		return nil, err
	}
	var d struct {
		TargetHealthDescriptions []struct {
			Target       struct{ Id string }
			TargetHealth struct{ State, Reason string }
		}
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	out := map[string]Target{}
	for _, t := range d.TargetHealthDescriptions {
		name := names[t.Target.Id]
		if name == "" {
			name = t.Target.Id
		}
		out[name] = Target{State: t.TargetHealth.State, Reason: t.TargetHealth.Reason}
	}
	return out, nil
}

func waitHealthy(ctx context.Context, c Config, health func(context.Context) (map[string]Target, error)) error {
	deadline := time.Now().Add(c.HealthyTimeout)
	for {
		t, err := health(ctx)
		if err == nil && t["web-a"].State == "healthy" && t["web-b"].State == "healthy" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("rung3: targets did not become healthy within %s (last: %v, err: %v); no fault was injected", c.HealthyTimeout, t, err)
		}
		if err := sleep(ctx, c.WaitPoll); err != nil {
			return err
		}
	}
}

// stopInstance is the fault: one EC2 StopInstances call (the call AWS FIS's aws:ec2:stop-instances
// makes; FIS itself is denied in this account by an organisation service control policy).
func stopInstance(ctx context.Context, c Config, id string) error {
	_, err := c.Exec(ctx, "aws", "ec2", "stop-instances", "--instance-ids", id, "--region", c.Region, "--output", "json")
	return err
}

func instanceState(ctx context.Context, c Config, id string) (string, error) {
	b, err := c.Exec(ctx, "aws", "ec2", "describe-instances", "--instance-ids", id, "--region", c.Region, "--query", "Reservations[0].Instances[0].State.Name", "--output", "text")
	if err != nil {
		return "", err
	}
	return "instance " + strings.TrimSpace(string(b)), nil
}

type rawProbe struct {
	at     time.Time
	took   time.Duration
	status int
	body   string
	err    string
}

type rawHealth struct {
	at time.Time
	t  map[string]Target
}

func probeLoop(ctx context.Context, c Config, url string, record func(rawProbe)) {
	tick := time.NewTicker(c.ProbeEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		go func() { // a slow request must not delay the next one
			start := time.Now()
			p := rawProbe{at: start}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := c.HTTP.Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return // the run ended; not a failure of the system under test
				}
				p.err = err.Error()
			} else {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
				resp.Body.Close()
				p.status, p.body = resp.StatusCode, strings.TrimSpace(string(b))
			}
			p.took = time.Since(start)
			record(p)
		}()
	}
}

func relativeProbes(raw []rawProbe, t0 time.Time) []Probe {
	out := make([]Probe, 0, len(raw))
	for _, p := range raw {
		out = append(out, Probe{AtMs: p.at.Sub(t0).Milliseconds(), DurationMs: p.took.Milliseconds(), Status: p.status, Body: p.body, Error: p.err})
	}
	sortProbes(out)
	return out
}

func relativeHealth(raw []rawHealth, t0 time.Time) []HealthSample {
	out := make([]HealthSample, 0, len(raw))
	for _, h := range raw {
		out = append(out, HealthSample{AtMs: h.at.Sub(t0).Milliseconds(), Targets: h.t})
	}
	return out
}

// destroyAndVerify tears everything down and then proves it, two independent ways: terraform's own
// state is empty, and the tagging API shows no resource still carrying this run's tag.
func destroyAndVerify(ctx context.Context, c Config, tf func(context.Context, ...string) ([]byte, error)) Cleanup {
	var cl Cleanup
	var derr error
	for attempt := 1; attempt <= 3; attempt++ {
		c.Log("terraform destroy (attempt %d)", attempt)
		if _, derr = tf(ctx, "destroy", "-auto-approve", "-input=false", "-no-color", "-var", "run_id="+c.RunID, "-var", "region="+c.Region); derr == nil {
			break
		}
	}
	cl.Destroyed = derr == nil
	if derr != nil {
		cl.Note = "terraform destroy failed: " + derr.Error()
	}
	if st, err := tf(ctx, "state", "list"); err == nil {
		cl.StateEmpty = strings.TrimSpace(string(st)) == ""
	}
	// The tagging API is eventually consistent; give a just-deleted resource time to drop out.
	for i := 0; i < 12; i++ {
		left, err := taggedLeftovers(ctx, c)
		if err == nil && len(left) == 0 {
			cl.TaggedLeftover = nil
			break
		}
		if err == nil {
			cl.TaggedLeftover = left
		}
		time.Sleep(c.DestroyPoll)
	}
	cl.Clean = cl.Destroyed && cl.StateEmpty && len(cl.TaggedLeftover) == 0
	if !cl.Clean && cl.Note == "" {
		cl.Note = fmt.Sprintf("destroyed=%v state_empty=%v leftover=%v", cl.Destroyed, cl.StateEmpty, cl.TaggedLeftover)
	}
	return cl
}

func taggedLeftovers(ctx context.Context, c Config) ([]string, error) {
	b, err := c.Exec(ctx, "aws", "resourcegroupstaggingapi", "get-resources", "--tag-filters", "Key=preflight-run,Values="+c.RunID, "--region", c.Region, "--output", "json")
	if err != nil {
		return nil, err
	}
	var d struct {
		ResourceTagMappingList []struct{ ResourceARN string }
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	var out []string
	for _, r := range d.ResourceTagMappingList {
		out = append(out, r.ResourceARN)
	}
	return out, nil
}

func sortProbes(p []Probe) { sort.Slice(p, func(i, j int) bool { return p[i].AtMs < p[j].AtMs }) }
