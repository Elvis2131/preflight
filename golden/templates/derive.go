package templates

import (
	"encoding/json"
	"fmt"

	"preflight/core"
	"preflight/ingest"
	"preflight/providers"
)

// FailureFault is the one declared fault each template's failure-mode fixture is
// derived under — chosen per template (see failureTargets) and never random.
type FailureFault = core.Fault

// failureTargets names, per template, the node whose loss the failure-mode fixture
// records: the stateful/entry component whose loss the template exists to survive or
// expose. A template with no entry here has no failure fixture and the test fails — a
// template without at least one failure-mode result is not shippable (the Card's rule).
var failureTargets = map[string]string{
	"three-tier-vpc": "aws_db_instance.db",
	"serverless-api": "aws_lambda_function.handler",
	"event-driven":   "aws_lambda_function.consumer",
}

// Derived is everything generated from a template's inputs by the real pipeline.
type Derived struct {
	IR       []byte
	Findings []byte
	Failure  []byte
}

// Derive runs a template through the production canvas path — ingest.IngestCanvas, then
// core.BuildFindings and core.Simulate — and returns the exact bytes a fixture holds.
// registry is the provider mapping registry the server itself would load.
func Derive(id string, registry providers.Registry) (Derived, error) {
	t, err := Load(id)
	if err != nil {
		return Derived{}, err
	}
	return DeriveFrom(t, registry)
}

// DeriveFrom is Derive over an already-loaded Template — the seam that lets a test
// tamper with an input and prove the byte comparison against the fixture really bites.
func DeriveFrom(t Template, registry providers.Registry) (Derived, error) {
	id := t.Meta.ID
	res, err := ingest.IngestCanvas(t.Canvas, registry, 1)
	if err != nil {
		return Derived{}, err
	}
	if res.IR == nil {
		return Derived{}, fmt.Errorf("template %q is below the Minimum Viable Graph threshold: %+v", id, res.Insufficient)
	}
	target, ok := failureTargets[id]
	if !ok {
		return Derived{}, fmt.Errorf("template %q has no failure-mode target registered", id)
	}
	prov := core.NewProvenance(core.KindDerived, "template-fixture")
	failure := core.Simulate(res.IR, t.Workload, []core.Fault{{Type: "node_loss", Target: target}}, prov)

	d := Derived{}
	if d.IR, err = marshal(res.IR); err != nil {
		return d, err
	}
	if d.Findings, err = marshal(core.BuildFindings(res.IR, t.Workload)); err != nil {
		return d, err
	}
	if d.Failure, err = marshal(failure); err != nil {
		return d, err
	}
	return d, nil
}

func marshal(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
