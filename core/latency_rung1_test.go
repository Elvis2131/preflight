//go:build live_rung1

package core_test

// PC-128, criterion 5: ONE predicted-vs-observed comparison for the Layer 3 latency model,
// against a real service in a local replica (Rung 1 of the validation ladder — "app
// behaviour", NOT cloud-native semantics or real capacity).
//
// Why this is not circular: the system under test is a real PostgreSQL server (the
// postgres:16-alpine image), not a server we wrote to follow the model. The only thing taken
// from the measurement is the service rate mu — and that is exactly the input a user DECLARES
// (Workload.Capacity); everything else is predicted by core.ComputeDegradedLatency and then
// compared with what the real server did.
//
// Method: a single pgbench client (-c 1 -j 1) running the select-only script is ONE server
// with a queue in front of it. With -R it issues Poisson arrivals at rate lambda; each
// transaction's sojourn time is the `time` column of pgbench's per-transaction log (-l), which
// already includes its schedule lag, so queueing is included and the DISTRIBUTION is observable, not only
// the mean (the mean is hostage to rare stalls — see the findings in docs/PREDICTED_VS_OBSERVED.md).
// mu and the service-time variance are calibrated first, unthrottled, from the same log.
//
//	go test -tags live_rung1 ./core/ -run Rung1_Latency -v -timeout 40m
//
// Needs Docker and a local postgres:16-alpine image. Not part of the default test run.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"preflight/core"
)

type stats struct {
	N      int     `json:"transactions"`
	MeanMS float64 `json:"mean_ms"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MaxMS  float64 `json:"max_ms"`
}

type rung1Row struct {
	Rho       float64 `json:"rho"`
	LambdaRPS float64 `json:"lambda_rps"`
	// Predictions by the real engine (M/M/1): mean from ComputeDegradedLatency, p50/p95/p99
	// from the exponential sojourn distribution the engine's own p95 = mean*ln20 assumes.
	PredictedMM1 stats `json:"predicted_mm1"`
	// M/G/1 (Pollaczek-Khinchine) mean using the service-time variance MEASURED in calibration.
	PredictedMG1MeanMS float64 `json:"predicted_mg1_mean_ms_measured_service_variance"`
	Runs               []stats `json:"observed_runs"`
	MedianOfRuns       stats   `json:"observed_median_of_runs"`
}

func docker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

var tpsRe = regexp.MustCompile(`tps = ([0-9.]+)`)

// pgbenchLog runs one pgbench client with a per-transaction log and returns the sojourn time
// of every transaction in ms (latency + schedule lag when rate-limited) plus reported tps.
func pgbenchLog(t *testing.T, container, script string, extra ...string) (sojournMS []float64, tps float64) {
	t.Helper()
	docker(t, "exec", container, "sh", "-c", "rm -f /tmp/pgb.*")
	args := []string{"exec", container, "pgbench", "-U", "postgres", "-c", "1", "-j", "1", "-l", "--log-prefix=/tmp/pgb"}
	if script == "" {
		args = append(args, "-S") // the built-in select-only script
	} else {
		args = append(args, "-f", script)
	}
	args = append(args, extra...)
	args = append(args, "bench")
	out := docker(t, args...)
	if m := tpsRe.FindStringSubmatch(out); m != nil {
		tps, _ = strconv.ParseFloat(m[1], 64)
	}
	logs := docker(t, "exec", container, "sh", "-c", "cat /tmp/pgb.*")
	for _, line := range strings.Split(logs, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		// Field 3 ("time") is the transaction's latency measured from its SCHEDULED start when
		// throttled, i.e. it ALREADY includes the schedule lag in the last field. Verified
		// 2026-10-02 against pgbench's own summary: "latency average" equalled the mean of this
		// column, not the mean of time+lag. (An earlier version of this harness added the lag
		// a second time and overstated every observed latency.)
		us, err := strconv.ParseFloat(f[2], 64)
		if err != nil {
			continue
		}
		sojournMS = append(sojournMS, us/1000)
	}
	if len(sojournMS) < 300 {
		t.Fatalf("only %d transactions in the pgbench log:\n%s", len(sojournMS), out)
	}
	return
}

func summarize(v []float64) stats {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	var sum float64
	for _, x := range c {
		sum += x
	}
	q := func(p float64) float64 { return c[int(p*float64(len(c)-1))] }
	return stats{N: len(c), MeanMS: sum / float64(len(c)), P50MS: q(0.50), P95MS: q(0.95), P99MS: q(0.99), MaxMS: c[len(c)-1]}
}

func predictMM1MS(t *testing.T, mu, lambda float64) stats {
	t.Helper()
	// The REAL engine: a two-component journey whose compute tier has effectively unbounded
	// capacity, so the whole latency is the database's. Offered load = the journey's peak rps.
	w := core.Workload{
		Capacity: map[string]float64{"compute_rps": 1e12, "managed_database_rps": mu},
		Journeys: []core.DeclaredJourney{{
			ID: "j", Name: "j", Path: []string{"app", "db"}, Protocol: "tcp", Port: 5432,
			Criticality: "tier1", PeakRPS: &lambda, SteadyRPS: &lambda,
		}},
	}
	est := estimateOf(t, core.ComputeDegradedLatency(buildFlowTestIR(true, true), w, nil, syntheticProv()))
	for _, c := range est.Components {
		if c.NodeID == "db" && c.MeanMS != nil {
			m := *c.MeanMS
			// M/M/1 sojourn time is exponential with that mean: quantile p = -mean*ln(1-p).
			return stats{MeanMS: m, P50MS: m * math.Ln2, P95MS: m * math.Log(20), P99MS: m * math.Log(100)}
		}
	}
	t.Fatalf("no database component latency in %+v", est.Components)
	return stats{}
}

func medianStats(runs []stats) stats {
	pick := func(f func(stats) float64) float64 {
		v := make([]float64, len(runs))
		for i, r := range runs {
			v[i] = f(r)
		}
		sort.Float64s(v)
		return v[len(v)/2]
	}
	return stats{
		N:      runs[0].N,
		MeanMS: pick(func(s stats) float64 { return s.MeanMS }), P50MS: pick(func(s stats) float64 { return s.P50MS }),
		P95MS: pick(func(s stats) float64 { return s.P95MS }), P99MS: pick(func(s stats) float64 { return s.P99MS }),
		MaxMS: pick(func(s stats) float64 { return s.MaxMS }),
	}
}

// Scenario 1: the built-in select-only script, ~0.07 ms service time. Included because it is the
// obvious first experiment — and because it shows the apparatus, not the model, is what it
// measures at that scale (a ~1 ms timer wake-up in the Docker VM dominates every latency).
func TestRung1_Latency_FastSelect(t *testing.T) {
	runRung1(t, "", "", 20, 15, "../docs/eval/latency-predicted-vs-observed-fast-select.json")
}

// Scenario 2: a real Postgres session holding for 10 ms of server-side work per request
// (SELECT pg_sleep(0.010)), a regime where timer noise is negligible next to the service time.
// The service time is near-deterministic, which M/M/1 does NOT assume — so a gap here is a
// property of the model's assumption, not of the apparatus.
func TestRung1_Latency_TenMillisecondService(t *testing.T) {
	runRung1(t, "ten-ms", "SELECT pg_sleep(0.010);", 20, 30, "../docs/eval/latency-predicted-vs-observed-10ms-service.json")
}

func runRung1(t *testing.T, scenario, sql string, calSeconds, runSeconds int, outFile string) {
	const container = "preflight-pc128-latency"
	exec.Command("docker", "rm", "-f", container).Run()
	docker(t, "run", "-d", "--rm", "--name", container, "-e", "POSTGRES_PASSWORD=pw", "postgres:16-alpine")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", container).Run() })
	for i := 0; ; i++ {
		if exec.Command("docker", "exec", container, "pg_isready", "-U", "postgres").Run() == nil {
			break
		}
		if i > 60 {
			t.Fatal("postgres did not become ready")
		}
		time.Sleep(time.Second)
	}
	docker(t, "exec", container, "createdb", "-U", "postgres", "bench")
	docker(t, "exec", container, "pgbench", "-U", "postgres", "-i", "-s", "1", "bench")
	script := ""
	if sql != "" {
		script = "/tmp/service.sql"
		docker(t, "exec", container, "sh", "-c", "echo '"+sql+"' > "+script)
	}
	pgbenchLog(t, container, script, "-T", "5") // warm-up, discarded

	// Calibrate mu and the service-time variance: single client, unthrottled, so each
	// transaction's latency IS its service time and throughput is 1 / mean service time.
	svc, mu := pgbenchLog(t, container, script, "-T", strconv.Itoa(calSeconds))
	svcStats := summarize(svc)
	var ss float64
	for _, x := range svc {
		ss += (x - svcStats.MeanMS) * (x - svcStats.MeanMS)
	}
	svcSD := math.Sqrt(ss / float64(len(svc)))
	scv := (svcSD / svcStats.MeanMS) * (svcSD / svcStats.MeanMS)
	t.Logf("calibrated mu = %.0f tps; service time mean %.4f ms, p50 %.4f, p99 %.4f, max %.2f ms, SCV %.2f (M/M/1 assumes SCV = 1)",
		mu, svcStats.MeanMS, svcStats.P50MS, svcStats.P99MS, svcStats.MaxMS, scv)

	const repeats = 3
	var rows []rung1Row
	for _, rho := range []float64{0.3, 0.5, 0.7, 0.8, 0.9} {
		lambda := rho * mu
		row := rung1Row{Rho: rho, LambdaRPS: lambda, PredictedMM1: predictMM1MS(t, mu, lambda)}
		// Pollaczek-Khinchine mean sojourn for M/G/1: E[S] + lambda*E[S^2] / (2*(1-rho)), with the
		// ACHIEVED utilisation rho = lambda*E[S] (the nominal rho is lambda over pgbench's reported
		// tps, which differs slightly from 1/E[S]) and E[S^2] from the measured service-time spread.
		es := svcStats.MeanMS / 1000
		es2 := (svcSD/1000)*(svcSD/1000) + es*es
		achievedRho := lambda * es
		row.PredictedMG1MeanMS = 1000 * (es + lambda*es2/(2*(1-achievedRho)))
		for r := 0; r < repeats; r++ {
			soj, _ := pgbenchLog(t, container, script, "-T", strconv.Itoa(runSeconds), "-R", fmt.Sprintf("%.2f", lambda))
			row.Runs = append(row.Runs, summarize(soj))
		}
		row.MedianOfRuns = medianStats(row.Runs)
		rows = append(rows, row)
		o := row.MedianOfRuns
		t.Logf("rho=%.1f | mean: M/M/1 %.3f  M/G/1 %.3f  observed %.3f | p50: pred %.3f obs %.3f | p95: pred %.3f obs %.3f | p99 obs %.3f max %.1f (ms, median of %d runs)",
			rho, row.PredictedMM1.MeanMS, row.PredictedMG1MeanMS, o.MeanMS, row.PredictedMM1.P50MS, o.P50MS, row.PredictedMM1.P95MS, o.P95MS, o.P99MS, o.MaxMS, repeats)
	}

	report := map[string]any{
		"ran_at":                  time.Now().UTC().Format(time.RFC3339),
		"system_under_test":       "postgres:16-alpine, pgbench select-only (-S), 1 client (-c 1 -j 1), open-loop Poisson arrivals (-R), per-transaction log (-l)",
		"docker_vm_cpus":          strings.TrimSpace(docker(t, "info", "--format", "{{.NCPU}}")),
		"calibrated_mu_tps":       mu,
		"calibrated_service_time": svcStats,
		"calibrated_service_scv":  scv,
		"repeats_per_level":       repeats,
		"rows":                    rows,
		"validation_rung":         "Rung 1 (local replica). Says nothing about cloud-native semantics or real capacity.",
	}
	b, _ := json.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(outFile, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
