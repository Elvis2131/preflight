# Browser end-to-end suite

These specs are the re-runnable evidence behind every "verified live in a browser" claim on the
Jira tickets. Each drives the **real** workspace in headless Chromium against a **real** assessd
— nothing is mocked — and **asserts**: a failed check prints what was expected and the process
exits non-zero.

```bash
cd canvas
npm run e2e                 # starts assessd + Vite on throwaway state, runs all specs, tears down
E2E_ONLY=02 npm run e2e     # only specs whose filename starts with 02
E2E_THEME=dark npm run e2e  # exercise every workflow with system dark appearance
```

Needs Go, Node, and a Playwright Chromium (`npx playwright install chromium`). To point at servers
you already run, set `UI_URL` and `API_URL` (nothing is spawned then). The runner seeds a fixture
pricing snapshot (`tests/e2e/seedpricing`) so report/re-price are verified without live AWS pricing.

| Spec | Ticket(s) | What it proves |
|---|---|---|
| `01-sg-authoring` | PC-137 | An SG authored on the canvas is evaluated: allowing rule passes the SG step; removing it fails *at* that step |
| `02-network-controls` | PC-138, PC-139, PC-113 | A canvas-built multi-hop design flows end to end; editing one route/NACL control in the Inspector blocks at exactly that step; out-of-range rule numbers are flagged |
| `03-failure-lab` | PC-131 | Build/run/save a multi-fault scenario; after the design changes, the same saved scenario is *re-evaluated* (not replayed); the report lists it |
| `04-groupings-and-icons` | PC-105, PC-109 | Official icons and the service palette render; the service canvas omits expanded infrastructure groupings |
| `05-report-export` | PC-122, PC-123 | Export PDF is a real, date-normalized PDF; re-price creates a new version and leaves the old report unchanged |
| `11-default-nacl` | PC-153 | The palette offers the default network ACL with its official icon; dropped inside a VPC it is tied to it by a `contained_in` edge; the Inspector explains it and switches from "assumed default" to "replaces the default" once a rule is authored |
| `06-service-library-and-toolbar` | AWS UI | Grouped service directory, official icons, service authoring and toolbar controls |
| `07-service-settings` | Service attributes | Clean sample layout retains its full graph; inline security rules change the real simulation; EC2 settings create real IAM, SG and subnet relationships; deleting services cleans up hidden edges |
| `09-network-templates` | Network architecture templates | Simple and enterprise designs are available through the API and UI; full graphs and clean layouts round-trip; assessed ingress and app hops work while unmodelled routing stays unknown |
| `10-traffic-playback` | Traffic animation | Packets move along directional edges; pause freezes them; sequential hops stop at actual rejections; fresh assessments replace old frames; reduced motion supports manual stepping; DNS-first network templates distinguish declared lookup from assessed HTTPS; status explains unknown capacity with a working edit action; playback never changes the saved graph or API input |
| `13-container-resizing` | VPC/subnet editing | Dragging corners expands real containers; minimap does not obscure handles; explicitly authored container sizes survive panel layout changes; resize metadata stays out of the API document and existing traffic still flows; simulation modes have no resize controls |

Selectors here are deliberately the app's own stable hooks (`data-testid`, `data-mode`,
`data-id`); if a spec breaks after a UI change, fix the spec's selector, never loosen an assertion.

`15-connection-picker` exercises real port drags, deferred creation, cancellation,
editing without changing IDs, duplicate handling, read-only simulation and popup bounds.

`16-clear-and-switch.mjs` checks one-click clearing, deferred sample replacement, cancellation and Escape, keyboard focus, empty-canvas loading, mode restrictions, and dark-mode confirmation styling.
