# Viewer (PC-90 / PC-91 / PC-92)

PRD §6's "secondary" web UI — a thin, read-only viewer. Reads the API only, authors
nothing. Not the canvas (PC-84): no drawing, no editing, no IR production. If this
ever grows a write/edit affordance, that's the canvas's territory, not this one's.

## What exists (PC-92: scorecard/timeline viewer)

- **`src/api.ts`** — the only place this app talks to assessd over HTTP. Calls
  `GET /sessions/{id}/versions` (added to `server/history.go` as PC-92's own
  groundwork — a real gap found starting this ticket: nothing could answer "how many
  versions does this session have," only "the latest" or "one specific number").
- **`src/timeline.ts`** — the one function permitted to turn a list of per-version
  `AssessResponse` into the table this app renders. Computes no average, count,
  ratio, or ranking — reviewed explicitly against the no-composite-score rule
  (CLAUDE.md §10, PC-19/PC-83) before being written, not just before merging. A
  finding that hasn't appeared yet in a given version renders as a real `null`, never
  coerced into a status string like `not_assessable` (a genuinely different, real
  status the server itself emits) — unit-tested and negative-controlled.
- **`src/App.tsx`** — a table: rows are finding IDs, columns are versions V1..Vn,
  cells show the server's own real status string plus (when present) the server's
  own real `DeltaKind` classification, color-coded. No composite score, trend line,
  or aggregate number anywhere.

## What exists (PC-91: graph/diff viewer)

- **`src/GraphView.tsx`** — renders one version's `graph` field (PC-81's real,
  deterministic SVG) verbatim via `dangerouslySetInnerHTML` — no `<img>`/data-URI
  round trip, no client-side transformation of any kind, this ticket's own first
  acceptance criterion ("renders PC-81's SVG output for a given version without
  modification"). This is trusted content: the SVG comes from `assessd`, a server
  this viewer is explicitly pointed at, not arbitrary third-party input. Below it, a
  plain list of that version's own `assurance_delta` entries (PC-19) — read directly,
  no new diff computation in the UI layer (this ticket's own second criterion). No
  write/edit action exists anywhere in this view (the third).
- **`App.tsx`**'s version selector defaults to the latest version and lets you step
  through V1..Vn; the diff section correctly reads "no prior version to diff
  against" for V1 rather than fabricating an empty diff.

## Verified, not just typechecked

Per this project's "run it, don't just typecheck it" discipline: launched against a
real running `assessd`, assessed `golden/aws-broken` then `golden/aws` under one
session (a real 2-version, 5-finding transition), and confirmed live in a real
browser (Playwright) that the table renders the exact real data — `unsatisfied ->
satisfied` classified `improvement`, `not_assessable -> satisfied` classified
`resolved_risk` (PC-83's own distinction, visibly preserved), the two structural
zone-kill findings staying `not_assessable`/`unchanged`. For PC-91: confirmed a real
`<svg>` element renders on the page showing the actual golden AWS architecture (real
resource addresses as node labels — `aws_db_instance.payments`, `aws_lb.payments`,
...), and that switching the version selector to V1 correctly shows "no prior version
to diff against" instead of an empty-but-implied-real diff. Screenshots taken and
inspected. Driver scripts scratch-only, not committed.

## Explicitly out of scope

- **No session history/browsing UI.** The session_id is typed in by hand; there is
  no "list my sessions" affordance (no such endpoint exists server-side either).

## Commands

```bash
npm install
npm run dev       # http://localhost:5173
npm run build      # tsc -b && vite build
npm test           # vitest
```

---

**Superseded by the workspace's Analyze mode (PC-104).** The scorecard timeline, graph and
diff in this standalone viewer now live in `canvas/src/analyze/` as the **Analyze** mode of
the single workspace, following the workspace's own session instead of a pasted
`session_id`. This directory is left in place (nothing was deleted) and still builds, but
new work belongs in `canvas/`.
