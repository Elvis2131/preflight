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

**Scope of the SVG fixture (PC-6, found by the first real CI run).** `golden_aws.svg` is only meaningful
where it was made: Graphviz 16.1.0 on macOS. Graphviz sizes every node from the width of its text, which
comes from the machine's fonts, so the same version draws a different diagram on Linux (a bare Linux
container with 16.1.0 differed in every node's width; apt's older Graphviz on the CI runner differed by
about 4,000 bytes). So `TestSVG_MatchesCheckedInGoldenFixture` runs only on that environment and reports
**SKIPPED, not passed** elsewhere. What every environment checks instead
(`TestSVG_StructureAndDeterminismInAnyEnvironment`): the SVG is well-formed XML, draws one node per IR node
and one edge per IR edge, names every node, and is byte-identical when rendered twice. The DOT input stays
byte-compared everywhere. The report fixtures (`golden/fixtures/aws.report.*`) embed a fixed placeholder
diagram for the same reason. Not claimed: that two different environments produce the same SVG bytes.

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

## The report diagram does not embed AWS icons (PC-109 decision)

The workspace canvas draws official AWS icons (`canvas/public/aws-icons/`, licence position in
`ICONS-LICENSE.md`); the server-rendered report SVG deliberately does **not**. Embedding them in
the Graphviz output would (a) make that output depend on external image files and their paths,
which the byte-stable golden SVG fixture and the deterministic-render requirement (NFR-1) cannot
tolerate, and (b) bloat every report with the same artwork. The report diagram therefore stays
label-based, and the golden SVG/HTML fixtures are byte-identical with or without the icon
directory (the full suite proves it). If a future ticket wants icons in the report, the
Graphviz version scope note in `render.go` applies and the fixtures would be regenerated and
hand-verified. Regenerated 2026-10-02 (PC-156: golden/aws's IR gained the three NAT gateways' aws_eip nodes and their edges once aws_eip became a mapped type — same Graphviz 16.1.0, so content-only).
