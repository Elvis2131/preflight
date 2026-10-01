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

Generated against Graphviz **16.1.0** (Homebrew, macOS arm64), 2026-09-25 (PC-111: regenerated after golden/aws's IR gained aws_route_table/aws_internet_gateway nodes and their routes_to/depends_on edges — same Graphviz version as the prior regeneration, so this update is content-only, not a version bump).

## PC-122: the PDF renderer does NOT get a golden fixture here

`render/pdf.go`'s `PDF` function shells out to **headless Chromium** the same way `SVG`
above shells out to `dot` (wkhtmltopdf was dropped: its repository was archived on
2023-01-02 and its last release was 2020-06-10). Its output is deliberately **not**
golden-fixture tested — a recorded decision, not an oversight. Chromium embeds a
`CreationDate`/`ModDate`; they were the only bytes that differed between two renders of the
same HTML (4 bytes, measured) and are fixed-width, so `render` rewrites them to a constant
and two renders in one environment ARE byte-identical (`render/pdf_test.go` proves it,
negative-controlled by bypassing the normalizer). But even with the dates fixed, PDF bytes
depend on the exact Chromium build (the `/Producer` string names it) and on which fonts the
machine has to subset, so a checked-in PDF would only be true on the machine that made it.
The HTML report (`core.RenderReportHTML`) IS golden-fixture tested and byte-stable
(`golden/fixtures/aws.report.html`); the PDF is a print of exactly that HTML, with the
Chromium version pinned and logged in CI like `dot -V`.
