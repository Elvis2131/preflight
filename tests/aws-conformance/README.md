# AWS conformance test tier (PC-119)

Two different test categories exist in this project, and this directory is only one of
them:

- **Internal regression tests** (`core/*_test.go`, everywhere else) prove the engine
  matches its own contracts — golden fixtures, negative controls.
- **AWS conformance tests** (here) prove the engine matches **documented AWS
  behaviour**. Every test cites the official AWS documentation URL it implements —
  enforced structurally by `tests/aws-conformance/harness.Verify`, not left to review
  discipline.

## Layout

```
tests/aws-conformance/
├── harness/        # the Spec format + Verify (citation-required lint)
├── networking/     # SG/NACL evaluation — PC-112/113 (not built yet)
├── routing/        # route tables, IGW/NAT — PC-111 (not built yet)
├── cidr/           # CIDR allocation — PC-106 (not built yet)
├── placement/      # subnet/AZ placement — PC-96 (not built yet)
├── failover/        # RDS Multi-AZ, ElastiCache automatic failover — built (this ticket's backfill)
├── pricing/        # Cost Engine — PC-100 (not built yet)
└── COVERAGE.md     # generated — see below
```

## Writing a conformance test

```go
spec := harness.Verify(t, harness.Spec{
	ID:            "AREA-RULE-NNN",
	Rule:          "plain-language AWS rule",
	Source:        "https://docs.aws.amazon.com/...",
	Scenario:      "the situation under test",
	Configuration: "the input this test constructs",
	Request:       "the action taken against it",
	Expected:      "the documented AWS behaviour being asserted",
})
// ... then a real behavioural assertion against Preflight's own engine, using spec.ID/
// spec.Rule/spec.Source in the failure message so a failing conformance test always
// names the rule and its source, per PC-119's own acceptance criterion.
```

`harness.Verify` fails the test immediately if any field is empty or `Source` isn't a
URL — this is the "a test missing its citation field fails lint/CI" criterion, enforced
by `go test ./...` itself (already CI's own gate), not a separate static-analysis tool.

## Coverage report

```bash
go run ./cmd/gen-conformance-report   # writes tests/aws-conformance/COVERAGE.md
```

Statically parses every `harness.Spec{...}` literal via `go/ast` (`cmd/
conformancescan`) rather than requiring an in-memory registry every test would need to
call into — area test packages run as separate `go test` processes and share no memory,
so static parsing is the only mechanism that can actually aggregate across all of them.
Regenerate after adding or removing a conformance test; do not hand-edit `COVERAGE.md`.

## Citation link check

```bash
go run ./cmd/check-conformance-links
```

Deliberately a periodic job (`.github/workflows/conformance-links.yml`, weekly, not
triggered on push/PR), not a per-commit CI gate — an AWS documentation page moving is a
real external fact this project doesn't control, and it must never fail an unrelated
commit's build.
