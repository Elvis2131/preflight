# Browser end-to-end suite

These specs are the re-runnable evidence behind every "verified live in a browser" claim on the
Jira tickets. Each drives the **real** workspace in headless Chromium against a **real** assessd
— nothing is mocked — and **asserts**: a failed check prints what was expected and the process
exits non-zero.

```bash
cd canvas
npm run e2e                 # starts assessd + Vite on throwaway state, runs all specs, tears down
E2E_ONLY=02 npm run e2e     # only specs whose filename starts with 02
```

Needs Go, Node, and a Playwright Chromium (`npx playwright install chromium`). To point at servers
you already run, set `UI_URL` and `API_URL` (nothing is spawned then). The runner seeds a fixture
pricing snapshot (`tests/e2e/seedpricing`) so report/re-price are verified without live AWS pricing.

| Spec | Ticket(s) | What it proves |
|---|---|---|
| `01-sg-authoring` | PC-137 | An SG authored on the canvas is evaluated: allowing rule passes the SG step; removing it fails *at* that step |
| `02-network-controls` | PC-138, PC-139, PC-113 | A canvas-built multi-hop design flows end to end; editing one route/NACL control in the Inspector blocks at exactly that step; out-of-range rule numbers are flagged |
| `03-failure-lab` | PC-131 | Build/run/save a multi-fault scenario; after the design changes, the same saved scenario is *re-evaluated* (not replayed); the report lists it |
| `04-groupings-and-icons` | PC-105, PC-109 | Server-derived Region/AZ groupings and public/private badges follow the model; groupings are read-only and never serialized; official icons and the service palette render |
| `05-report-export` | PC-122, PC-123 | Export PDF is a real, date-normalized PDF; re-price creates a new version and leaves the old report unchanged |
| `11-default-nacl` | PC-153 | The palette offers the default network ACL with its official icon; dropped inside a VPC it is tied to it by a `contained_in` edge; the Inspector explains it and switches from "assumed default" to "replaces the default" once a rule is authored |

Selectors here are deliberately the app's own stable hooks (`data-testid`, `data-mode`,
`data-id`); if a spec breaks after a UI change, fix the spec's selector, never loosen an assertion.
