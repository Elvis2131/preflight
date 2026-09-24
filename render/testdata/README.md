# Golden render fixtures

`golden_aws.dot`/`golden_aws.svg` are checked-in output, not a cache — the real guard
behind PC-81's determinism claim. Without these, "byte-identical SVG output" was only
ever checked against itself within a single test run (two invocations, same process,
same installed Graphviz) — it could never catch the CI runner's Graphviz version
drifting between two *separate* runs, since both invocations in that one run would
still agree with each other even if the version had changed since the last run. These
files are what actually makes that drift fail loudly instead of passing silently.

## Regenerating

```bash
go run ./cmd/gen-render-fixture   # see cmd/gen-render-fixture/main.go
```

(Or by hand: run `core.RenderDOT` + `render.SVG` against `golden/aws` and write the
two files directly — that's all the generator does.)

## If `render/render_fixture_test.go` fails

Two different, real causes produce the identical symptom (byte mismatch) — tell them
apart before regenerating anything:

- **`TestRenderDOT_MatchesCheckedInGoldenFixture` fails**: `core.RenderDOT` is pure Go
  with zero external dependency, so this is ALWAYS a real code change to `RenderDOT`
  itself (or to `ingest`/`providers/aws` changing the golden IR it's fed) — never a
  Graphviz version issue, since this test never invokes `dot`.
- **`TestSVG_MatchesCheckedInGoldenFixture` fails but the DOT test above passes**:
  the DOT text going into Graphviz is unchanged, so this is the actual gap this
  fixture exists to catch — the installed `dot` binary's version differs from
  whatever generated `golden_aws.svg` (check `.github/workflows/ci.yml`'s own `dot -V`
  log line against the version noted below). This is real, expected maintenance, not
  a bug: regenerate, diff the new SVG against the old one, confirm the change is
  cosmetic (layout/rendering differences from the new Graphviz version), and commit
  the update along with a note of which Graphviz version it now reflects — the same
  "don't trust a regeneration silently, verify it" discipline `golden/fixtures/`
  already established for IR/findings.

Generated against Graphviz **16.1.0** (Homebrew, macOS arm64), 2026-09-24.
