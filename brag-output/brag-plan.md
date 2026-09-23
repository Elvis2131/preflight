# Brag Plan: Preflight

## What is this app?
Preflight is a server engineers (or their AI agents) call while authoring cloud
infrastructure — it takes the current IaC plus a declared workload and returns a
versioned assessment: compliance with evidence, an architecture graph, and
failure-mode analysis with real fault simulation, so you find out what survives a
region loss before you deploy, not after.

## The angle
This isn't an "AI writes your infra" pitch — it's the opposite: infrastructure is
being AI-generated faster than anyone can review it, so Preflight is the thing that
checks it, with receipts. The strongest material isn't a feature list, it's the
project's own honesty: it caught and published a real bug in itself — a database
with zero redundancy that would have been reported as *resilient* — and fixed it
before anyone shipped on the wrong answer. That's the brag: not "we're right," but
"we show our work, including the one time we weren't."

## Hook (first 2-3 seconds)
Wordmark on a near-black field, then the tagline typing in beneath it — restrained,
confident, a little ominous:
"Don't just generate infrastructure —"
beat —
"prove what happens when it fails."

## Key moments (the middle)
- The canvas: a golden-vocabulary node (e.g. "Managed Database") dragged from the
  palette onto the graph, wired to a load balancer with a `depends_on` edge, the raw
  CanvasDocument JSON panel flickering open beside it — this is a real screen, not a
  mockup of one.
- Findings resolving with their provenance tags visible: `unsatisfied`,
  `not_assessable`, `satisfied` — each stamped `stated` / `derived` / `assumed` /
  `llm_reasoned` — the tri-state result the whole product is built around, never a
  bare pass/fail.
- A region-loss simulation: the architecture graph going dark node by node, landing
  on a real stat line pulled from the project's own verified results — "28 nodes
  traced. 4 stateful resources severed. 0% capacity survives." — then a beat
  confirming it exports as a runnable AWS FIS chaos experiment, not just a claim.

## Outro / punchline
The self-correction beat, played completely straight: a single card stating the real
bug the project found in itself — a database with no standby, reported as
*resilient* — and that it was caught and fixed. Then cut to the wordmark and a short
tagline. No triumphant music swell. The restraint is the point.

## User flow worth showing
1. **Entry** — an engineer (or agent) declares a resource on the canvas and wires it
   into the graph — this is the actual `canvas/` React app, not an imagined UI.
2. **Key action** — that graph gets assessed: findings come back with a status
   (`satisfied` / `unsatisfied` / `not_assessable`) and a provenance tag on every
   single one.
3. **Result** — a fault is simulated against the same graph (region loss), and the
   blast radius is reported as a real, countable number — then exported as a chaos
   experiment someone could actually run.

## Tone
- Preset: `polished`
- Creative direction: a quiet, confident infrastructure tool that would rather say
  "I don't know" than guess — and isn't afraid to show the one time it got something
  wrong.
- Interpretation: slow, generous holds; one idea per scene; no bullet-point energy;
  confidence expressed through restraint and specificity (real numbers, real
  screens) rather than hype language or exclamation points.

## Format: landscape — 1920x1080
## Duration: 20s

## Visual identity (from the project)
- Background: `#0b0a14` (near-black, faint violet cast) for brand/type scenes;
  `#f8f7fc` (near-white, the real canvas app's own background) for the UI-recreation
  scene, so that scene reads as the actual product screen.
- Accent (primary): `#7e14ff` (the wordmark's violet)
- Accent (secondary): `#47bfff` (the wordmark's electric blue, used sparingly for
  the "satisfied" / success state)
- Warning/attention accent (for `unsatisfied` / severed-node states): a restrained
  amber, not red — this product never screams
- Neutral/`not_assessable` accent: a muted slate, deliberately unexciting — it is
  neither a pass nor a fail
- Text: `#f5f3ff` on dark scenes, `#0f0a1f` on the light UI scene
- Display font: Inter (clean geometric sans — matches the project's own
  system-ui/Segoe/Roboto stack without the genericness of defaulting to it verbatim)
- Body font: system-ui fallback stack, same as the real app
- Strongest visual element: the violet/blue gradient mark from `canvas/public/favicon.svg`,
  used as the brand anchor in the hook and outro; the real canvas UI (palette +
  graph + JSON panel) as the centerpiece "show the thing" scene

## Share copy (draft)
Infra gets generated faster than anyone can review it. Preflight checks it before
you deploy — and publishes the one time its own answer was wrong.

## Audio direction
- Role: sparse professional accents over a low, restrained bed — never a hype build
- Music: a minimal, low-key electronic/ambient bed, low BPM, no aggressive drops
- Music treatment: starts near-silent under the hook, stays low and steady through
  the middle, a very slight lift (not a swell) under the punchline card, soft fade
  under the outro hold
- Music cue guidance: to be detected at composition time (no bundled preset
  targeted); align the graph-node "severed" beats in Scene 3 loosely to the beat
  grid rather than hard-syncing every hit
- Audio-reactive treatment: none — this tone earns restraint, not a reactive glow
- SFX posture: sparse. A dry, quiet click on the node-drop and edge-connect; a soft,
  low tone (not an alarm) on each severed node in the region-loss beat; no whoosh,
  no riser, no cheer
- Audio-coupled moments: the hook tagline typing in (very soft key ticks, if at all);
  the node-drag-and-connect in Scene 2; the sequential node "severed" reveal in
  Scene 3
- Restraint rule: no music swell, no triumphant sting anywhere — including under the
  punchline, which is a confession, not a win

## Storyboard

### Scene 1 — Hook — 3s
Near-black field (`#0b0a14`). The Preflight mark (the violet/blue gradient shape)
fades in centered, small, quiet. Below it, the tagline types in across two lines:
"Don't just generate infrastructure —" / "prove what happens when it fails." Wordmark
"Preflight" appears last, smallest text on screen, bottom-left.
Sequential/interaction: yes — the two tagline lines arrive in sequence, first line
settles fully before the second begins.
Audio intent: quiet, a little tense — the calm before a claim.
Audio-coupled idea: very soft key-tick under the typing text, barely audible.
Music: low ambient bed, just starting.
Transition mood: soft crossfade → Scene 2

### Scene 2 — The canvas (entry → evaluate) — 6s
Cut to the real UI: light background (`#f8f7fc`), the golden-vocabulary palette on
the left exactly as the real app renders it (Compute, Managed Database, Cache, Load
Balancer, Queue/Stream, etc.), a node card dragging from the palette onto the graph
canvas, snapping into place, then a `depends_on` edge drawing from it to an existing
Load Balancer node. The CanvasDocument JSON panel slides in from the right, showing
real-shaped JSON. Dissolve into a findings list: three rows resolving in sequence —
`unsatisfied` (amber), `not_assessable` (slate), `satisfied` (blue) — each with a
small provenance tag (`derived`, `stated`, `assumed`) beside it.
Sequential/interaction: yes — simulate the drag-and-drop, then the edge draw, then
the three findings rows arriving one by one, each holding before the next appears.
Audio intent: focused, procedural — the sound of a system actually checking
something, not celebrating.
Audio-coupled idea: a dry, soft click on node-drop and on edge-connect; a very quiet
distinct tick per findings row as it resolves.
Music: steady, low, unchanged from Scene 1.
Transition mood: clean hard cut → Scene 3

### Scene 3 — Region loss — 6s
Cut to a rendered architecture graph on the dark background. Nodes go dark one by
one in a deliberate sequence (not a flash) as the simulated region loss propagates.
A stat line builds beneath it, arriving as three short beats: "28 nodes traced." /
"4 stateful resources severed." / "0% capacity survives." A final small line settles
underneath, understated: "Exported as a runnable chaos experiment."
Sequential/interaction: yes — nodes darken in a traced sequence (not simultaneous),
then the three stat lines arrive one at a time, each held to its full reading time
before the next.
Audio intent: sober, a little heavier — this is the "what actually breaks" moment.
Audio-coupled idea: one soft, low (non-alarm) tone per node as it goes dark, loosely
on the beat grid; a slightly firmer tone on the final "0% capacity survives" line.
Music: unchanged, low — no swell here despite the content; the restraint is
deliberate.
Transition mood: slow crossfade → Scene 4

### Scene 4 — Punchline + outro — 5s
A single card, centered, dark background: "We predicted a broken database would
fail over. It wouldn't have. We caught it — and fixed it." Hold. Then dissolve to
the Preflight mark and wordmark, centered, with the tagline small beneath:
"Evidenced assurance. Not a guess." Long hold on empty space before end.
Sequential/interaction: none — one static card, then one static outro frame; the
stillness is the point.
Audio intent: quiet confidence, not triumph — same volume as the rest of the video,
no button-up moment.
Audio-coupled idea: none.
Music: very slight lift under the punchline card, then a soft fade to silence under
the outro hold.
Transition mood: soft crossfade in, long hold, fade to black.

**Music mood for this video:** deadpan / restrained — low, steady, almost
unnoticeable; the antithesis of a hype-reel build.
**Audio summary:** A quiet, procedural bed under the whole video with sparse, dry,
motion-matched clicks and tones — no swells, no drops, no triumphant sting, even at
the punchline — because the video's entire claim is that this product doesn't
oversell what it found.
