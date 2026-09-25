// Package harness is PC-119: the AWS conformance test tier's shared format and
// enforcement. This is the ONE place that decides what a conformance test is,
// reused by every area under tests/aws-conformance/ (networking, routing, cidr,
// placement, failover, pricing) — not duplicated per area.
//
// The distinction this tier exists to draw, verbatim from the Card: internal
// regression tests prove the engine matches its OWN contracts (golden fixtures,
// negative controls — everything core/*_test.go already does); conformance tests
// prove the engine matches DOCUMENTED AWS BEHAVIOUR. A conformance test that doesn't
// cite where that documented behaviour comes from is not distinguishable from a
// regression test that merely restates what the code already does — Verify below is
// what makes the citation load-bearing rather than decorative.
package harness

import (
	"fmt"
	"strings"
	"testing"
)

// Spec is one conformance test's fixed metadata shape — the Card's own field list,
// verbatim: test ID, AWS rule in plain language, official source URL, scenario,
// configuration, request, expected behaviour.
type Spec struct {
	// ID follows the Card's own example convention: AREA-RULE-NNN, e.g.
	// "FAILOVER-RDS-MULTIAZ-001", "SG-STATEFUL-001", "NACL-ORDER-001".
	ID string

	// Rule states, in plain language, the AWS behaviour this test verifies — not a
	// restatement of what the code does, but what AWS itself documents doing.
	Rule string

	// Source is the official AWS documentation URL this Rule is verified against.
	// Required and non-empty (enforced below) — this is the field that makes a
	// conformance test a conformance test rather than an ordinary regression test.
	Source string

	// Scenario names the situation under test in one sentence.
	Scenario string

	// Configuration describes the input (Terraform/IR shape) this test constructs.
	Configuration string

	// Request describes the action taken against that configuration (a query, a
	// simulated fault, an evaluation call) — "n/a" is an acceptable, honest value for
	// a test with no distinct request step (e.g. a pure static-mapping check), but the
	// field must be set explicitly, not left as Go's zero value.
	Request string

	// Expected states the documented AWS behaviour this test asserts against.
	Expected string
}

// validate is Verify's actual logic, split out as a plain function (no *testing.T)
// specifically so spec_test.go can assert on its return value directly — nesting a
// testing.T that's expected to fail inside another test is exactly the kind of
// t.Run-subprocess trickery this project avoids elsewhere (see core/boundary_test.go's
// own choice to shell out to a real `go build` rather than fight the test framework).
func validate(spec Spec) error {
	if spec.ID == "" {
		return fmt.Errorf("conformance spec has no ID")
	}
	// Ordered, not map-iterated: deterministic which field a spec missing several
	// gets reported for first, and this codebase's own lint rule (forbidigo via
	// .golangci.yml, extended here in spirit) already treats unsorted map iteration
	// feeding output as a smell to avoid on principle, not just inside core/.
	type field struct {
		name  string
		value string
	}
	for _, f := range []field{
		{"Rule", spec.Rule},
		{"Source", spec.Source},
		{"Scenario", spec.Scenario},
		{"Configuration", spec.Configuration},
		{"Request", spec.Request},
		{"Expected", spec.Expected},
	} {
		if f.value == "" {
			return fmt.Errorf("%s: missing required field %q", spec.ID, f.name)
		}
	}
	if !strings.HasPrefix(spec.Source, "https://") && !strings.HasPrefix(spec.Source, "http://") {
		return fmt.Errorf("%s: Source %q is not a URL — a conformance test must cite an official AWS documentation link, not a paraphrase", spec.ID, spec.Source)
	}
	return nil
}

// Verify is this tier's citation-required lint, enforced the same way every other
// structural rule in this codebase is: by actually failing the test, which is what
// makes `go test ./...` (already CI's own gate — PC-119's own acceptance criterion,
// "fails lint/CI") catch it, not a separate static-analysis tool this project would
// have to introduce and maintain.
//
// Returns spec unchanged so the caller can embed spec.ID/spec.Rule/spec.Source in its
// own failure message when the BEHAVIOURAL assertion (never made here — Verify only
// checks the metadata is complete and well-formed) fails.
func Verify(t *testing.T, spec Spec) Spec {
	t.Helper()
	if err := validate(spec); err != nil {
		t.Fatal(err)
	}
	return spec
}
