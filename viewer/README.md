# Viewer (PC-90 / PC-92)

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

## Verified, not just typechecked

Per this project's "run it, don't just typecheck it" discipline: launched against a
real running `assessd`, assessed `golden/aws-broken` then `golden/aws` under one
session (a real 2-version, 5-finding transition), and confirmed live in a real
browser (Playwright) that the table renders the exact real data — `unsatisfied ->
satisfied` classified `improvement`, `not_assessable -> satisfied` classified
`resolved_risk` (PC-83's own distinction, visibly preserved), the two structural
zone-kill findings staying `not_assessable`/`unchanged`. Screenshot taken and
inspected. Driver script scratch-only, not committed.

## Explicitly out of scope for this ticket

- **No graph/diff view.** That's PC-91, blocked on PC-81 (server-rendered SVG, not
  yet built) — this app has nothing to render for that yet.
- **No session history/browsing UI.** The session_id is typed in by hand; there is
  no "list my sessions" affordance (no such endpoint exists server-side either).

## Commands

```bash
npm install
npm run dev       # http://localhost:5173
npm run build      # tsc -b && vite build
npm test           # vitest
```
