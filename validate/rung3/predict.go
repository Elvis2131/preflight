// Package rung3 holds the PREDICTION half of PC-25's predicted-vs-observed experiment: what
// Preflight (and AWS's own documentation) says will happen when AWS FIS stops one of two web
// instances behind an ALB. It touches no cloud and holds no credential. The observing half, which
// does, lives in cmd/runnerd/internal/rung3 (P3, the only process allowed credentials, ADR-003).
//
// The prediction is written to predictions.json and committed BEFORE the experiment is run, so the
// comparison cannot be adjusted after the fact.
//
// Scope, stated rather than implied: the experiment injects NODE loss (FIS aws:ec2:stop-instances on
// one instance). It is not an availability-zone failure. Preflight's zone-kill findings are written
// for the golden bundle's own subnets (core/findings_builder.go) and report not_assessable for
// any other bundle, so they predict nothing here; the node_loss simulation is what applies.
package rung3

import (
	"fmt"
	"os"
	"strings"

	"preflight/core"
	"preflight/server"
)

// Health-check settings declared in terraform/main.tf. TestPredict_MirrorsTerraform keeps these in
// step with the Terraform so the derived bound cannot drift from what is actually deployed.
const (
	HealthCheckIntervalSeconds = 10
	UnhealthyThresholdCount    = 2
)

// FaultTarget is the instance FIS stops (terraform: aws_fis_experiment_template.stop_az_a).
const FaultTarget = "aws_instance.web_a"

// Statement is one checkable claim made before the run.
type Statement struct {
	ID         string          `json:"id"`
	Claim      string          `json:"claim"`
	Source     string          `json:"source"`
	Provenance core.Provenance `json:"provenance"`
	// Observable says how the run will confirm or refute it. "not_observed" means the run does not
	// test this claim (it is recorded so it is not mistaken for validated).
	Observable string `json:"observable"`
}

// Prediction is everything claimed before the run.
type Prediction struct {
	Experiment string      `json:"experiment"`
	Fault      string      `json:"fault"`
	Statements []Statement `json:"statements"`
	// EngineSilentOn lists questions the experiment answers but Preflight makes no prediction about.
	EngineSilentOn []string `json:"engine_silent_on"`
}

// Predict assesses the bundle with the real pipeline and simulates the fault.
func Predict(bundleDir, workloadPath string) (Prediction, error) {
	store, err := server.OpenStore(":memory:")
	if err != nil {
		return Prediction{}, err
	}
	defer store.Close()
	const session = "rung3-predict"
	assessed, err := server.Assess(store, server.AssessRequest{SessionID: session, BundleDir: bundleDir, WorkloadPath: workloadPath})
	if err != nil {
		return Prediction{}, fmt.Errorf("assess %s: %w", bundleDir, err)
	}
	sim, err := server.Simulate(store, server.SimulateRequest{SessionID: session, VersionNumber: assessed.VersionNumber,
		Faults: []core.Fault{{Type: "node_loss", Target: FaultTarget}}})
	if err != nil {
		return Prediction{}, err
	}

	flows := map[string]core.JourneyFlowResult{}
	for _, f := range sim.FlowDetail {
		flows[f.JourneyID] = f
	}
	viaB, okB := flows["via-b"]
	viaA, okA := flows["via-a"]
	if !okB || !okA {
		return Prediction{}, fmt.Errorf("simulation returned no flow for the declared journeys: %v", sim.FlowDetail)
	}

	engine := core.NewProvenance(core.KindDerived, "preflight node_loss simulation of "+FaultTarget)
	docs := core.NewProvenance(core.KindDerived, "AWS ELB docs (target-group-health-checks) applied to the declared health_check settings")

	p := Prediction{
		Experiment: "stop one of two ALB targets with AWS FIS (aws:ec2:stop-instances)",
		Fault:      FaultTarget,
	}
	if viaB.Flows && !viaA.Flows {
		p.Statements = append(p.Statements, Statement{
			ID:         "P1",
			Claim:      "After the fault, the surviving path internet -> ALB -> web-b still flows and the path through web-a no longer does: the service stays reachable and every request is served by web-b.",
			Source:     "Preflight /simulate node_loss: flow_detail via-b flows, via-a blocked",
			Provenance: engine,
			Observable: "HTTP requests to the ALB succeed after the detection window and are all answered by web-b",
		})
	} else {
		p.Statements = append(p.Statements, Statement{
			ID: "P1", Claim: fmt.Sprintf("engine result was unexpected (via-b flows=%v, via-a flows=%v)", viaB.Flows, viaA.Flows),
			Source: "Preflight /simulate node_loss", Provenance: engine, Observable: "not_observed"})
	}
	if v := fmt.Sprint(sim.Verdict.Value); sim.Verdict.State == core.AssessmentStateAssessed && v != "" {
		p.Statements = append(p.Statements, Statement{
			ID: "P2", Claim: "The simulation verdict for losing one instance is \"" + v + "\": no entry point loses its path to stateful storage.",
			Source: "Preflight /simulate node_loss: verdict", Provenance: engine,
			Observable: "not_observed (a structural verdict; the run observes request outcomes, not storage reachability)"})
	}
	bound := HealthCheckIntervalSeconds*UnhealthyThresholdCount + HealthCheckIntervalSeconds
	p.Statements = append(p.Statements,
		Statement{
			ID: "P3",
			Claim: fmt.Sprintf("The ALB stops sending traffic to the stopped instance within %d s of it ceasing to respond: %d consecutive failed checks at one check every %d s, plus at most one interval of phase. Until then some requests (about half, with two targets) fail.",
				bound, UnhealthyThresholdCount, HealthCheckIntervalSeconds),
			Source:     "AWS docs: health checks run every HealthCheckIntervalSeconds; a target is out of service after UnhealthyThresholdCount consecutive failures",
			Provenance: docs,
			Observable: "last failed request time relative to the first failed request; probe failure share during the window",
		},
		Statement{
			ID:         "P4",
			Claim:      "Once the instance reaches the stopped state the target group reports it as unused with reason Target.InvalidState (not unhealthy).",
			Source:     "AWS docs: target health status table (unused / Target.InvalidState: target is in the stopped state)",
			Provenance: docs,
			Observable: "describe-target-health for web-a after the stop completes",
		})
	p.EngineSilentOn = []string{
		"how long the failover takes (the engine has no health-check detection model; rto_seconds is declared as a requirement but no RTO finding is produced for this bundle)",
		"how many requests fail while the ALB detects the loss",
		"availability-zone loss (zone-kill findings exist only for the golden bundle's own subnets)",
	}
	return p, nil
}

// MirrorsTerraform reports whether main.tf still declares the health-check values this package
// derives its bound from.
func MirrorsTerraform(mainTF string) error {
	b, err := os.ReadFile(mainTF)
	if err != nil {
		return err
	}
	s := string(b)
	for _, want := range []string{
		fmt.Sprintf("interval            = %d", HealthCheckIntervalSeconds),
		fmt.Sprintf("unhealthy_threshold = %d", UnhealthyThresholdCount),
		`resource_arns  = [aws_instance.web_a.arn]`,
	} {
		if !strings.Contains(s, want) {
			return fmt.Errorf("%s no longer declares %q", mainTF, want)
		}
	}
	return nil
}
