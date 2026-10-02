package core_test

// PC-128: Layer 3 latency/saturation tests. Every expected number below is computed by
// hand from the M/M/1 closed form (mean = 1000/(mu-lambda) ms, p95 = mean*ln 20), not
// copied from a first run.

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"preflight/core"
)

func f64(v float64) *float64 { return &v }

func latencyWorkload(path string, dbCap float64) core.Workload {
	return core.Workload{
		Capacity: map[string]float64{"compute_rps": 200, "managed_database_rps": dbCap},
		Journeys: []core.DeclaredJourney{{
			ID: "j", Name: "j", Path: []string{"app", path}, Protocol: "tcp", Port: 5432,
			Criticality: "tier1", PeakRPS: f64(100), SteadyRPS: f64(50),
		}},
	}
}

func estimateOf(t *testing.T, res []core.JourneyLatency) core.JourneyLatencyEstimate {
	t.Helper()
	if len(res) != 1 {
		t.Fatalf("want one journey result, got %d", len(res))
	}
	if res[0].Result.State != core.AssessmentStateAssessed {
		t.Fatalf("want assessed, got not_assessable: %s", res[0].Result.Reason)
	}
	est, ok := res[0].Result.Value.(core.JourneyLatencyEstimate)
	if !ok {
		t.Fatalf("value is %T, want JourneyLatencyEstimate", res[0].Result.Value)
	}
	return est
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// app: lambda=100, mu=200 -> 1000/100 = 10 ms. db: lambda=100, mu=150 -> 1000/50 = 20 ms.
// Journey mean = 30 ms. p95(app) = 10*ln(20) = 29.9573... ms.
func TestComputeDegradedLatency_HandVerifiedBaseline(t *testing.T) {
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), latencyWorkload("db", 150), nil, syntheticProv()))

	if est.MeanMS == nil || !near(*est.MeanMS, 30) {
		t.Fatalf("journey mean = %v, want 30 ms", est.MeanMS)
	}
	if len(est.Components) != 2 {
		t.Fatalf("components = %+v, want app and db", est.Components)
	}
	app, db := est.Components[0], est.Components[1]
	if app.NodeID != "app" || !near(*app.MeanMS, 10) || !near(*app.P95MS, 10*math.Log(20)) || !near(app.Utilization, 0.5) {
		t.Errorf("app = %+v, want mean 10 ms, p95 %.4f ms, utilization 0.5", app, 10*math.Log(20))
	}
	if db.NodeID != "db" || !near(*db.MeanMS, 20) {
		t.Errorf("db = %+v, want mean 20 ms", db)
	}
}

// The result is framed as conditional and lists every declared input beside it, and the
// model states it has no seed because it has no randomness.
func TestComputeDegradedLatency_ConditionalFramingAndInputsListed(t *testing.T) {
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), latencyWorkload("db", 150), nil, syntheticProv()))

	if !strings.Contains(est.Statement, "Under the declared inputs") {
		t.Errorf("statement %q must be framed as conditional on the declared inputs", est.Statement)
	}
	var names []string
	for _, in := range est.Inputs {
		names = append(names, in.Name)
		if in.Source != "stated" {
			t.Errorf("input %q has source %q, want stated — nothing here may be assumed or derived", in.Name, in.Source)
		}
	}
	want := []string{"journey j peak_rps", "journey j steady_rps", "Workload.Capacity[compute_rps]", "Workload.Capacity[managed_database_rps]"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("inputs = %v, want %v", names, want)
	}
	if len(est.Assumptions) == 0 || est.Seed != core.LatencySeedNote {
		t.Errorf("assumptions/seed missing: %+v", est)
	}
}

// Every missing declared input is a named not_assessable — never defaulted.
func TestComputeDegradedLatency_MissingInputsAreNamedNotAssessable(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	cases := map[string]struct {
		mutate func(w *core.Workload)
		want   string
	}{
		"no capacity for a path component": {func(w *core.Workload) { delete(w.Capacity, "managed_database_rps") }, "managed_database_rps"},
		"no peak_rps":                      {func(w *core.Workload) { w.Journeys[0].PeakRPS = nil }, "peak_rps"},
		"no steady_rps":                    {func(w *core.Workload) { w.Journeys[0].SteadyRPS = nil }, "steady_rps"},
	}
	for name, c := range cases {
		w := latencyWorkload("db", 150)
		c.mutate(&w)
		res := core.ComputeDegradedLatency(ir, w, nil, syntheticProv())[0].Result
		if res.State != core.AssessmentStateNotAssessable || !strings.Contains(res.Reason, c.want) {
			t.Errorf("%s: got state %q reason %q, want not_assessable naming %q", name, res.State, res.Reason, c.want)
		}
	}
}

// Offered load >= declared capacity: no steady state exists, so no finite latency is
// reported — the saturated component is named instead.
func TestComputeDegradedLatency_SaturationHasNoInventedNumber(t *testing.T) {
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), latencyWorkload("db", 100), nil, syntheticProv()))
	if est.MeanMS != nil || est.SaturatedAt != "db" {
		t.Fatalf("want saturated at db with no mean, got mean=%v saturated_at=%q", est.MeanMS, est.SaturatedAt)
	}
	if !est.Components[1].Saturated || est.Components[1].MeanMS != nil || est.Components[1].P95MS != nil {
		t.Errorf("db component = %+v, want saturated with no mean/p95", est.Components[1])
	}
}

// The point of Layer 3: after a failure, the survivor carries the load and latency rises.
// Path app -> (db | db2), peak 100 rps split evenly. Baseline: each db sees 50 rps, mu=150
// -> 10 ms. db2 killed: db sees 100 rps -> 1000/50 = 20 ms. Journey mean 10+10=20 -> 10+20=30.
func TestComputeDegradedLatency_FailureShiftsLoadAndRaisesLatency(t *testing.T) {
	ir := buildTwoDatabaseIR()
	w := latencyWorkload("db|db2", 150)

	base := estimateOf(t, core.ComputeDegradedLatency(ir, w, nil, syntheticProv()))
	if base.MeanMS == nil || !near(*base.MeanMS, 20) {
		t.Fatalf("baseline mean = %v, want 20 ms", base.MeanMS)
	}
	degraded := estimateOf(t, core.ComputeDegradedLatency(ir, w, map[string]bool{"db2": true}, syntheticProv()))
	if degraded.MeanMS == nil || !near(*degraded.MeanMS, 30) {
		t.Fatalf("degraded mean = %v, want 30 ms (db alone carries 100 rps)", degraded.MeanMS)
	}
}

// Killing the only path leaves nothing to estimate: not_assessable, saying where it is
// blocked — not a latency, and not a pass.
func TestComputeDegradedLatency_JourneyThatDoesNotFlowHasNoLatency(t *testing.T) {
	res := core.ComputeDegradedLatency(buildFlowTestIR(true, true), latencyWorkload("db", 150), map[string]bool{"db": true}, syntheticProv())[0].Result
	if res.State != core.AssessmentStateNotAssessable || !strings.Contains(res.Reason, "does not flow") {
		t.Fatalf("got %q / %q, want not_assessable explaining the journey does not flow", res.State, res.Reason)
	}
}

// Deterministic: same inputs -> identical output, and a changed declared input changes
// it (so the determinism is not an artefact of ignoring inputs — the negative control).
func TestComputeDegradedLatency_DeterministicAndInputSensitive(t *testing.T) {
	ir := buildFlowTestIR(true, true)
	a := core.ComputeDegradedLatency(ir, latencyWorkload("db", 150), nil, syntheticProv())
	b := core.ComputeDegradedLatency(ir, latencyWorkload("db", 150), nil, syntheticProv())
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same inputs produced different output")
	}
	c := core.ComputeDegradedLatency(ir, latencyWorkload("db", 160), nil, syntheticProv())
	if reflect.DeepEqual(a, c) {
		t.Fatal("changing a declared capacity did not change the output — the model is ignoring its inputs")
	}
}

// Regression for the readiness gate PC-128 found broken: a parallel-path journey's
// "db|db2" element must be checked member by member, not looked up as one node ID.
func TestEvaluateJourneyLoadReadiness_ParallelPathMembersEachNeedCapacity(t *testing.T) {
	ir := buildTwoDatabaseIR()
	w := latencyWorkload("db|db2", 150)
	if got := core.EvaluateJourneyLoadReadiness(ir, w, w.Journeys[0], syntheticProv()); got.State != core.AssessmentStateAssessed {
		t.Fatalf("parallel-path journey with every capacity declared must be ready, got: %s", got.Reason)
	}
	delete(w.Capacity, "managed_database_rps")
	got := core.EvaluateJourneyLoadReadiness(ir, w, w.Journeys[0], syntheticProv())
	if got.State != core.AssessmentStateNotAssessable || !strings.Contains(got.Reason, "managed_database_rps") {
		t.Fatalf("missing member capacity must be named, got %q / %q", got.State, got.Reason)
	}
}

// ---- Declared service-time variability (SCV) — added after the Rung 1 comparison ----

func dbOf(t *testing.T, est core.JourneyLatencyEstimate) core.ComponentLatency {
	t.Helper()
	for _, c := range est.Components {
		if c.NodeID == "db" {
			return c
		}
	}
	t.Fatalf("no db component in %+v", est.Components)
	return core.ComponentLatency{}
}

// SCV = 1 is exponential service, so the Pollaczek-Khinchine mean must reproduce M/M/1
// exactly: db lambda=100, mu=150 -> 1000/(150-100) = 20 ms either way.
func TestServiceTimeSCV_One_ReproducesMM1(t *testing.T) {
	w := latencyWorkload("db", 150)
	w.ServiceTimeSCV = map[string]float64{"managed_database_rps": 1}
	db := dbOf(t, estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), w, nil, syntheticProv())))
	if db.MeanMS == nil || !near(*db.MeanMS, 20) {
		t.Fatalf("SCV=1 mean = %v, want exactly the M/M/1 20 ms", db.MeanMS)
	}
}

// SCV = 0 is perfectly regular service (M/D/1). Hand-computed: rho = 100/150 = 2/3 and
// E[S^2] = 1/mu^2, so the waiting time is Wq = lambda*E[S^2]/(2*(1-rho)) = 100/(22500*2*(1/3))
// = 100/15000 s = 6.6667 ms; adding the 6.6667 ms service time gives T = 13.3333 ms
// (against 20 ms under M/M/1).
func TestServiceTimeSCV_Zero_IsMD1_AndWithholdsP95(t *testing.T) {
	w := latencyWorkload("db", 150)
	w.ServiceTimeSCV = map[string]float64{"managed_database_rps": 0}
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), w, nil, syntheticProv()))
	db := dbOf(t, est)
	if db.MeanMS == nil || math.Abs(*db.MeanMS-40.0/3) > 1e-9 {
		t.Fatalf("SCV=0 mean = %v, want 13.3333 ms (M/D/1)", db.MeanMS)
	}
	if db.P95MS != nil || !db.SCVDeclared || db.ServiceTimeSCV != 0 {
		t.Errorf("a declared SCV must withhold p95 (no closed form) and be reported: %+v", db)
	}
	// The undeclared tier keeps the M/M/1 numbers: app lambda=100, mu=200 -> 10 ms, with p95.
	var app core.ComponentLatency
	for _, c := range est.Components {
		if c.NodeID == "app" {
			app = c
		}
	}
	if app.MeanMS == nil || !near(*app.MeanMS, 10) || app.P95MS == nil || app.SCVDeclared || app.ServiceTimeSCV != 1 {
		t.Errorf("an undeclared component must stay M/M/1 with its p95: %+v", app)
	}
	if est.MeanMS == nil || math.Abs(*est.MeanMS-(10+40.0/3)) > 1e-9 {
		t.Errorf("journey mean = %v, want 10 + 13.3333", est.MeanMS)
	}
	found := false
	for _, in := range est.Inputs {
		if in.Name == "Workload.ServiceTimeSCV[managed_database_rps]" && in.Value == 0 && in.Source == "stated" {
			found = true
		}
	}
	if !found {
		t.Errorf("the declared SCV must be listed next to the result: %+v", est.Inputs)
	}
}

// No SCV declared anywhere: output is exactly what it was before this input existed.
func TestServiceTimeSCV_Absent_ChangesNothing(t *testing.T) {
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), latencyWorkload("db", 150), nil, syntheticProv()))
	for _, c := range est.Components {
		if c.SCVDeclared || c.ServiceTimeSCV != 1 || c.P95MS == nil {
			t.Errorf("%s: undeclared SCV must be the M/M/1 assumption with a p95: %+v", c.NodeID, c)
		}
	}
	if est.MeanMS == nil || !near(*est.MeanMS, 30) {
		t.Errorf("baseline mean = %v, want the hand-verified 30 ms", est.MeanMS)
	}
}

func TestServiceTimeSCV_NegativeIsRejected(t *testing.T) {
	w := core.Workload{SchemaVersion: "1.0.0", Name: "w", Criticality: "tier1", DataClassification: "internal", Regions: []string{"eu-west-1"},
		ServiceTimeSCV: map[string]float64{"managed_database_rps": -0.1}}
	if err := w.Validate(); err == nil {
		t.Fatal("a negative squared coefficient of variation is meaningless and must be rejected")
	}
}

// Ties the engine to the recorded evidence: for every utilisation in the Rung 1 run on a real
// Postgres with a 10 ms service time (docs/eval/latency-predicted-vs-observed-10ms-service.json),
// the engine — given the MEASURED service rate and SCV as declared inputs — must reproduce the
// Pollaczek-Khinchine prediction recorded there, within 1% (the harness used the nominal rho
// where the engine uses the achieved one).
func TestServiceTimeSCV_MatchesTheRecordedRung1Evidence(t *testing.T) {
	raw, err := os.ReadFile("../docs/eval/latency-predicted-vs-observed-10ms-service.json")
	if err != nil {
		t.Skipf("recorded evidence not present: %v", err)
	}
	var ev struct {
		ServiceTime struct {
			MeanMS float64 `json:"mean_ms"`
		} `json:"calibrated_service_time"`
		SCV  float64 `json:"calibrated_service_scv"`
		Rows []struct {
			LambdaRPS float64 `json:"lambda_rps"`
			MG1MeanMS float64 `json:"predicted_mg1_mean_ms_measured_service_variance"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	if len(ev.Rows) == 0 || ev.ServiceTime.MeanMS == 0 {
		t.Fatalf("evidence file has no usable rows: %+v", ev)
	}
	mu := 1000 / ev.ServiceTime.MeanMS // declared capacity = 1 / mean service time
	for _, row := range ev.Rows {
		lambda := row.LambdaRPS
		w := core.Workload{
			Capacity:       map[string]float64{"compute_rps": 1e12, "managed_database_rps": mu},
			ServiceTimeSCV: map[string]float64{"managed_database_rps": ev.SCV},
			Journeys: []core.DeclaredJourney{{ID: "j", Name: "j", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432,
				Criticality: "tier1", PeakRPS: &lambda, SteadyRPS: &lambda}},
		}
		db := dbOf(t, estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), w, nil, syntheticProv())))
		if db.MeanMS == nil || math.Abs(*db.MeanMS-row.MG1MeanMS)/row.MG1MeanMS > 0.01 {
			t.Errorf("lambda=%.2f: engine %.3f ms, recorded P-K %.3f ms", lambda, *db.MeanMS, row.MG1MeanMS)
		}
	}
}
