# Canvas (PC-85)

## Local development and appearance

Start the current backend from the repository root with `go run ./cmd/assessd`,
then run `npm run dev` in `canvas/`. The frontend defaults to port 8080 for the
backend. For a different port, put `VITE_ASSESSD_URL=http://localhost:8091` in
`canvas/.env.local` and start the backend with `PREFLIGHT_ASSESSD_PORT=8091`.
Restart Vite after changing its environment. Use the current backend source:
older running binaries can answer `/healthz` while returning 404 for `/templates`
and `/catalog/services`. Template failures now show an explanation and a retry.

The moon/sun button at the bottom of the navigation rail switches appearance.
The first visit follows the system theme; an explicit choice persists in local
storage. Theme changes preserve the active canvas, form values and simulation.

Drag-and-drop architecture authoring canvas — PC-84's epic, PC-85's own shell-only
scope. React + TypeScript + Vite + `@xyflow/react` 12 (the confirmed library choice
per PC-85's own Card, validated earlier against the aws-resilience-simulator reference
implementation — studied for design, never vendored, per ADR-004's own vendor-vs-port
reasoning).

## Connections

Drag from a service's output port to another service's input port. A popup at the
destination names both services and asks for the connection type. Selecting a type
creates the connection; Escape, Cancel or clicking outside discards it. Pending
connections never enter the saved architecture document. Click a connection or its
label to change its type while retaining its ID and endpoints. Drawing the same
connection again opens its editor instead of adding a duplicate. Editing remains
available only in Design. Connections use rounded paths and larger hit areas.

## Service view and attributes

Templates open in a service view that arranges the main services along their traffic
paths. Network and access resources remain in the design and are configured from the
selected service's **Service settings** panel. **Arrange** restores the service-flow
layout. The expanded Infrastructure view and its toggle have been removed.

Select an EC2 or container service to assign an IAM role, security groups and subnets.
Choose an existing resource or use **+ New** to create and assign one. Click an assigned
resource's name to edit its details. Security group rules use named source groups;
subnet settings expose VPC, route-table, ACL and gateway configuration. Database and
cache services keep their subnet-group relationships. Shared-resource edits apply to
every attached service; unassigning a resource retains it for other services.

Selecting a service focuses its settings and closes the Workload panel. **Workload**
reopens workload requirements and capacity without losing their values. Moving cards
in service view changes only layout, leaving subnet membership intact. Resources
explicitly dragged from the library remain visible, including configuration resources.

`serviceConfiguration.ts` authors ordinary resource nodes and graph edges; there is
no second attribute store or new API contract. Assessment, simulation and JSON export
always receive the complete document. IAM role assignments use the existing identity
relationship; this editor does not author IAM policy documents. Layout positions,
view state and card summaries stay in the browser. Browser coverage in
`e2e/07-service-settings.mjs` verifies inline edits against the real backend, including
blocking and restoring a journey by changing a security group rule.

## What exists

- **`src/goldenVocabulary.ts`** — the canonical node/edge vocabulary, mirrored
  deliberately from `preflight/core/ir.go`'s own frozen `core.NodeType`/`core.EdgeType`
  enums (11 node types, 6 edge types). One source of truth for the mirror; every other
  file imports from here.
- **`src/types.ts`** — `CanvasDocument`, the one serialized shape this app ever
  produces (nodes + edges + capability, no UI-only fields).
- **`src/serialize.ts`** — the one function permitted to turn React Flow's live state
  into a `CanvasDocument`. Unit-tested (`src/serialize.test.ts`) against React Flow's
  own real `NodeBase`/`EdgeBase` fields (position, selected, dragging, handles —
  verified directly against `@xyflow/system`'s type declarations, not guessed) to
  prove none of them leak through.
- **`src/GoldenNode.tsx`** — one custom node component for every golden type
  (deliberately one, not eleven — a per-type component list would just move
  CLAUDE.md's own "giant service conditional" anti-pattern into the canvas).
- **`src/App.tsx`** — the palette (drag source), the canvas (drop target + edge
  drawing with an explicit edge-type selector), a live `CanvasDocument` JSON preview
  panel, and (PC-88) click-a-node-to-select + "Kill selected node".
- **`src/api.ts`** — the ONLY place this app calls assessd's HTTP routes (PC-88): thin
  `fetch` wrappers around `POST /sessions/{id}/canvas` (PC-86) and `POST /simulate`
  (PC-82/PC-88's `node_loss` fault) — no fault logic of its own, matching this
  project's "MCP tools and HTTP routes must call identical underlying functions"
  discipline extended to this second client.

## Verified, not just typechecked

Per this project's own "run it, don't just typecheck it" discipline: the app was
actually launched (`npm run dev`) and driven with a real, headless Chromium browser
(Playwright) — dragged two real node types from the palette onto the canvas, drew a
real edge between them with a non-default edge type selected, and confirmed the live
`CanvasDocument` JSON output was exactly correct (right node types/labels, right edge
type, `from`/`to` correctly mapped from React Flow's own `source`/`target`, zero
UI-only fields). Screenshots taken and inspected, not just the JSON. The driver script
itself was scratch-only and not committed — the unit tests in `serialize.test.ts` are
what's kept.

## Explicitly out of scope for this ticket

- **No node-level capability form.** Nodes carry an empty `capability: {}` — a later
  capability-editing ticket owns giving these real, typed, per-node-type structure.
  (PC-87 gave the WORKLOAD its own form; per-node capability is still unaddressed.)
- **No persistence.** Refreshing the page loses the canvas — this is a shell, not a
  saved-document product yet.

## Backend wiring (PC-86)

PC-86 landed `POST /sessions/{id}/canvas` (`preflight/server/canvas.go`): the same
`CanvasDocument` this app's `serialize.ts` produces is now the sixth frozen wire
contract (`contracts/canvas.schema.json`), consumed server-side by
`ingest.IngestCanvas` and run through the identical assessment pipeline `/assess`
already uses. Live-verified end to end: a real browser-captured `CanvasDocument`
(`contracts/samples/canvas.sample.json`) posted via curl against a running `assessd`
produced real findings honestly reflecting the canvas's entered/missing capability
data. This app itself still makes no HTTP calls of its own — PC-86 only proves the
server-side half is ready for a future ticket to wire the "Assess" button up to it.

## NFR form (PC-87)

`src/WorkloadForm.tsx` + `src/workloadTypes.ts` — a form over the exact
`workload.schema.json` shape (`core.Workload`), not a new or looser schema: same
`name`/`criticality`/`data_classification`/`regions`/`compliance_profiles`/
`requirements`/`capacity` fields, same semantics. Two things this ticket's own
Conversation named as easy to get wrong, both handled explicitly:

- **A blank capacity field is `capacity_unknown`, never zero.** `buildWorkload`
  (`WorkloadForm.tsx`) drops a capacity row from the output map entirely when its
  value is left blank — it never becomes a `0` entry. Unit-tested and
  negative-controlled (`WorkloadForm.test.ts`: temporarily made a blank value coerce
  to `0`, confirmed the test caught it, restored). Verified live too: filling in the
  form with `app_node_rps` left blank, killing a node, and reading back `/simulate`'s
  own real `capacity` field showed `capacity_unknown: "app_node_rps" was not
  declared...` — not just a form-side check.
- **Hard vs. preference requirements are visually and functionally distinct**, not a
  dropdown: separate red/blue panels, and `rank` only ever appears (and is only ever
  sent) for a preference requirement — a hard requirement can never carry a `rank`
  key at all, matching `core.Requirement`'s own `excluded_if` tag.

The server's own `AssessCanvasRequest` gained a `workload` field (an inline
`core.Workload`, alongside the existing `workload_path`) since a browser form has no
way to hand the server a server-filesystem path — validated with the identical
`Workload.Validate()` call the file-based path already used (`server/canvas.go`).

## Follow traffic

Click **Follow traffic** in Design to assess the current architecture and replay a
declared journey on the canvas. In Simulate or Failure Lab it replays the current
assessment, including faults. Pick a journey, play/pause, change playback speed,
replay, or use **Next hop** and the numbered steps. The viewport focuses on the chosen
path; **Focus path** restores it after panning or zooming. Internet entry points are visible;
packets follow arrows from source to destination and the current services carry hop
badges. Parallel branches play together. A rejected hop has a stationary stop marker
and the backend's reason; an unmodelled hop never receives a successful packet.

Traffic checks project `/simulate`'s returned `Hops`/`GroupIndex`/`Allowed` fields.
It computes no reachability, capacity, security decision or latency. Display-only
entry cards and hop edges never enter the authored document or assessment input.
Changing the design or workload invalidates the old replay. Reduced-motion users
can step through the same results without moving packets. Playback speed is visual
pacing, not network timing.

The simple and enterprise network templates show Internet clients connected to Route 53
for DNS, then separately to the ALB for HTTPS. Saved `design_dns_client` / `design_alias`
values describe this lookup. The blue DNS replay step illustrates declared architecture
and explicitly says DNS resolution is unassessed; it never creates an `Allowed` result
or sends HTTPS through Route 53. The client and DNS connection also remain visible in
Design, using the declared internet origin without adding a resource to the backend graph.
The following traffic checks use the declared per-hop ports and stop at the current
backend's modelling limits.
Existing single-hop checks remain individually selectable.
**Ports along this journey** in Workload preserves the template's per-hop ports and
lets you edit them, with a blank field using the journey's declared default port.

**Baseline first (PC-161).** A simulation result opens with "Does traffic flow before the fault?", the
server's own answer for each declared journey: flowed before and after, never carried traffic even
before the fault (with the hop and the reason), broken by the fault, running on a fallback, or could not
be checked. A journey that was already blocked is never reported as having survived, and the headline
reads "No survival verdict: the design was already broken before the fault". With no journey declared the
panel says traffic flow was not checked. In the Failure Lab and the report, a scenario lists such journeys
separately from the ones the fault broke. "Could not be checked" (an unreadable or unmodelled input) is
different from "blocked" (a real deny); the canvas shows what the server says and decides neither.

Simulation status uses plain-language labels for the engine's structural verdict,
disconnected destinations and failure cascade. An unaffected structural result does
not mean every traffic or capacity check passed. Missing capacity is shown as unset,
with **Set capacity** opening Workload; instance counts never substitute for tested
throughput. **Technical details** retains the original server values and reasons.
Editing the design or workload hides the previous inputs' status until reassessment.

## Failure/cascade animation (PC-89)

A real gap found before writing any animation code, worth recording since it changed
this ticket's actual scope: `cascade[]`'s "returned order" was alphabetical by node
ID (`sort.Strings` in `core/simulate.go`), carrying zero structural meaning — a node
six hops from the kill point could animate before one directly adjacent to it, purely
because its ID sorted earlier. Fixed at the source: `core/internal/analyse/zoneloss.go`
now has `CascadeOrder`, which orders cascade members by real hop distance from the
killed node (a structural, deterministic, topology fact — never a timing or rate claim;
Layer 2/3 remain untouched, per this ticket's own explicit boundary). `core/simulate.go`
uses it for both `cascade` and `severed_paths`.

`App.tsx`'s `applySimResult` reveals `cascade[]` members one at a time, in that exact
order, on a fixed `STEP_DELAY_MS` interval — a UI pacing choice only, displayed as a
step COUNT ("revealing step 2 of 3"), never a duration or rate. No intermediate state
is invented: a not-yet-revealed member simply stays unstyled until its own step.

**Verified live**, not just typechecked: built a linear chain (load balancer -> compute
-> database -> cache) deliberately chosen so alphabetical order (`cache-4`,
`container_workload-2`, `managed_database-3`) visibly disagrees with real hop order.
Killed the compute node and confirmed both the real backend response
(`cascade: [container_workload-2, managed_database-3, cache-4]` — hop order, not
alphabetical) and the live DOM at increasing delays: only the killed node styled at
~50ms, the database added at ~600ms, the cache added at ~1100ms — a genuinely staged
reveal, not an instant diff. Driver script scratch-only, not committed.

## Structured error handling (PC-95)

`src/api.ts`'s `APIError` mirrors `server.APIError`'s wire shape (`{error_code,
message}`) — `postJSON` parses it when present and throws `APIError` instead of a
bare `Error`, falling back to the old plain-text behavior for anything that doesn't
parse (an older/differently-versioned server). `describeSimError` is the one place
that turns any caught error into display text; it branches on `error_code ===
"session_not_found"` for a distinct, actionable message, proving the structured
signal is actually read somewhere rather than only theoretically available.
Unit-tested and negative-controlled (`api.test.ts`).

## Kill-node simulation (PC-88)

Clicking a node selects it; "Kill selected node" re-assesses the current canvas
(`POST /sessions/{id}/canvas`) to get a fresh version, then calls `POST /simulate`
with `{type: "node_loss", target: <clicked node's id>}` — a fault type added to
`core.Simulate` specifically for this ticket (region_loss's whole-graph semantics
couldn't answer "what if this ONE node dies"; see `core/simulate.go`'s own doc
comment). The response's `severed_paths`/`cascade` are painted onto the graph exactly
as returned — killed node in red, `severed_paths` nodes greyed with a dashed border,
any other `cascade` member (unreachable but not itself a stateful target) in a lighter
grey — never reinterpreted or summarised differently for the canvas.

Requires assessd running locally (`go run ./cmd/assessd` from the repo root, so
`workload_path` — defaulting to `golden/workload.yaml` in the toolbar — resolves) on
its default port 8080; `server.WithCORS` (added for this ticket) is what lets the
Vite dev origin call it directly from the browser.

**Verified, not just typechecked, twice**: once with a minimal two-node graph
(load balancer -> database, killed the database, got back `severed_paths: [that
node]` exactly), and once reproducing the golden architecture's own real chokepoint
shape (one compute tier feeding both a database and a cache, matching golden/aws's
actual EKS -> RDS / EKS -> ElastiCache structure) — killing the shared compute node
correctly severed both downstream stateful nodes, matched visually (both greyed,
both edges dashed) against the exact `severed_paths` the live backend returned, not
a guessed or assumed set. Driver scripts were scratch-only, not committed.

## Commands

```bash
npm install
npm run dev       # http://localhost:5173
npm run build      # tsc -b && vite build
npm test           # vitest
```

Requires a working Node.js — if your system `node`/`npm` are broken (a common cause:
a Homebrew library version mismatch), point PATH at a working install explicitly
rather than assuming the default resolves correctly.

In Design, **Clear design** clears all services and connections, resets the template workload, and removes assessment results. Selecting a sample over an existing design opens an in-app confirmation naming the sample and explaining what will be replaced. **Keep current design** or Escape cancels; **Switch sample** loads it. An empty canvas loads a sample directly.
