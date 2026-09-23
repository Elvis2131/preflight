# Hyperframes Composition Brief: Preflight

## Objective
Create a short launch-style brag video for Preflight, an architecture assurance
control loop for AI-authored infrastructure.

## Output
- Composition directory: `brag-output/composition/`
- Rendered video: `brag-output/brag.mp4`
- Format: landscape — 1920x1080
- Duration: 20 seconds

## Source Material
- Project root: `/Users/larteyelvis/Documents/Developer/preflight`
- Primary files read: `README.md`, `CLAUDE.md`, `docs/PREDICTED_VS_OBSERVED.md`,
  `canvas/src/App.tsx`, `canvas/src/GoldenNode.tsx`, `canvas/src/goldenVocabulary.ts`,
  `canvas/src/index.css`, `canvas/public/favicon.svg`
- Product name: Preflight
- Tagline / strongest claim: "Don't just generate infrastructure — prove what
  happens when it fails."
- Key UI or visual moment to recreate: the real `canvas/` React app — a left-hand
  palette of golden-vocabulary node types (Compute, Container Workload, Managed
  Database, Cache, Load Balancer, Queue/Stream, Object Store, DNS, Network Boundary,
  Identity, External Dependency), a drag-and-drop graph canvas, and a live
  `CanvasDocument` JSON panel — plus a findings list with `satisfied` /
  `unsatisfied` / `not_assessable` states, each carrying a provenance tag (`stated`
  / `derived` / `assumed` / `llm_reasoned` / `observed`).
- Copy that must appear verbatim:
  - "Don't just generate infrastructure — prove what happens when it fails."
  - "28 nodes traced."
  - "4 stateful resources severed."
  - "0% capacity survives."
  - "Exported as a runnable chaos experiment."
  - "We predicted a broken database would fail over. It wouldn't have. We caught
    it — and fixed it."
  - "Evidenced assurance. Not a guess."
  - "Preflight"

## Creative Direction
- Tone preset: `polished`
- Creative direction: a quiet, confident infrastructure tool that would rather say
  "I don't know" than guess — and is willing to show the one time it got something
  wrong.
- Interpretation: 4 scenes, slow generous holds (3-6s each), one idea per scene, no
  bullet-point energy or hype language. Confidence expressed through restraint and
  specific real numbers/real screens rather than exclamation points or a music
  swell — including at the punchline, which is a confession, not a win.
- Angle: Infrastructure is now generated faster than anyone can review it by hand.
  Preflight is the thing that checks it — deterministically, with evidence on every
  claim, and a tri-state result (`not_assessable` is a first-class outcome, never a
  forced pass/fail). The video's strongest material isn't a feature list — it's
  that this project caught and published a real bug in itself (a database with zero
  redundancy that would have been reported as resilient) and fixed it before anyone
  shipped on the wrong answer.
- Hook: near-black field, the Preflight mark fades in quiet and small, then the
  tagline types in across two held lines.
- Outro / punchline: a single still card confessing the real self-found bug, held,
  then a dissolve to the wordmark + a short tagline on empty space. No triumphant
  sting.
- Avoid:
  - Generic SaaS language ("streamline your workflow" etc. — never use it)
  - Abstract filler visuals (no stock-motion gradients, no generic particle systems)
  - A composite/single "score" anywhere on screen — this product explicitly never
    produces one; do not invent one for the video
  - Turning the punchline into a celebration — it must read as calm and honest, not
    triumphant

## Visual Identity
- Background (brand/type scenes): `#0b0a14` (near-black, faint violet cast)
- Background (UI-recreation scene): `#f8f7fc` (the real canvas app's own light bg)
- Accent (primary): `#7e14ff` (violet, from `canvas/public/favicon.svg`)
- Accent (secondary): `#47bfff` (electric blue, from the same mark; use for the
  `satisfied` state)
- Attention accent (`unsatisfied` / severed-node state): a restrained amber —
  never red, this product doesn't shout
- Neutral accent (`not_assessable` state): a muted slate — deliberately
  unexciting, it is neither a pass nor a fail
- Text: `#f5f3ff` on dark scenes, `#0f0a1f` on the light UI scene
- Display font: Inter (or closest available clean geometric sans)
- Body font: system-ui fallback stack (matches the real app: `system-ui, "Segoe UI", Roboto, sans-serif`)
- Visual references from the project:
  - The violet/blue gradient mark at `canvas/public/favicon.svg` — use as the brand
    anchor in the hook and outro (recreate its color relationships; the file itself
    is an app icon asset, not required verbatim)
  - The real canvas UI structure from `canvas/src/App.tsx` / `GoldenNode.tsx`: a
    ~220px left palette listing the 11 golden node types, a graph canvas with
    rounded-rect node cards (white fill, `#94a3b8` border, `#2563eb` selected-state
    border), a dark JSON panel (`#0f172a` background, `#e2e8f0` text) sliding in
    from the right

## Storyboard
Use the storyboard in `brag-output/brag-plan.md` as the creative contract. Full
per-scene text, sequencing, and audio intent are specified there — summary below.

1. Hook — 3s — mark fades in, tagline types across two held lines, wordmark settles
   bottom-left last.
2. The canvas (entry → evaluate) — 6s — real canvas UI: node dragged from palette,
   snapped, wired with a `depends_on` edge to an existing Load Balancer node,
   CanvasDocument JSON panel slides in; dissolve to 3 findings rows resolving in
   sequence (`unsatisfied` amber / `not_assessable` slate / `satisfied` blue), each
   with its provenance tag.
3. Region loss — 6s — architecture graph nodes go dark in a traced sequence; three
   stat lines arrive one at a time ("28 nodes traced." / "4 stateful resources
   severed." / "0% capacity survives."); a smaller line settles: "Exported as a
   runnable chaos experiment."
4. Punchline + outro — 5s — one still confession card, held, then dissolve to mark +
   wordmark + "Evidenced assurance. Not a guess." on a long empty hold.

## Audio
- Audio role: sparse professional accents over a low, restrained bed — never a
  hype build, including at the punchline
- Audio arc: near-silent under the hook, steady and low through the middle, a very
  slight (not a swell) lift under the punchline card, soft fade to silence under
  the outro hold
- Music: `assets/music/happy-beats-business-moves-vol-12-by-ende-dot-app.mp3`
  (already copied into `composition/assets/music/`) at low volume (0.15-0.22 —
  restrained, per the `polished`/deadpan-adjacent treatment this plan calls for,
  not the standard 0.3-0.4 bed level)
- Music treatment: fade in under Scene 1, hold steady and low through Scenes 2-3,
  a very slight (not dramatic) lift under the Scene 4 punchline card, fade to
  silence under the final outro hold
- Music cue guidance: bundled preset at
  `assets/music/cues/happy-beats-business-moves-vol-12-by-ende-dot-app.music-cues.json`
  (+ matching `.md`) — use `strongCues` sparingly (1-2 locks max, given the
  restrained tone) for the Scene 2→3 transition and/or the Scene 4 dissolve to the
  outro mark; use the beat grid loosely, if at all, for the Scene 3 node-darkening
  sequence — but only if it doesn't rush the "28 nodes traced." stat-line reading
  time. Prefer natural timing over forcing a beat lock when it would conflict with
  the reading-time floor.
- Audio-reactive treatment: none — this tone earns restraint, not a reactive glow;
  do not wire any visual element to RMS/frequency data
- Audio-coupled moments:
  - Scene 1 hook tagline typing — optional, very soft key-tick only, easy to omit
    entirely if it reads as noisy
  - Scene 2 node-drop and edge-connect — a dry, quiet interface click
  - Scene 2 findings rows resolving — a very quiet, distinct tick per row
  - Scene 3 node-darkening sequence — one soft, low (non-alarm) tone per node,
    loosely on the beat grid; a slightly firmer tone under "0% capacity survives."
  - Scene 4 — no SFX; let the still card and the outro hold carry in silence
    against the fading music bed
- SFX selection guidance: `assets/sfx/interface/drop_001.ogg` for gentle
  element-landing moments (node drop, findings rows); `assets/sfx/interface/bong_001.ogg`
  reserved for at most one restrained accent (candidate: the Scene 3 final stat
  line, or the Scene 4 dissolve to the outro mark — pick whichever reads calmer in
  context, not both). Both files are already copied into
  `composition/assets/sfx/interface/`. Do not reach for anything from the
  `impact/`, `casino/`, or `glitch` families — none of that energy matches this
  tone.
- SFX analysis guidance: read `<hyperframes-skill-dir>/assets/sfx/sfx-analysis.md`
  (or the equivalent bundled with the hyperframes-creative skill) and prefer
  low/medium high-frequency-risk picks throughout — this is a `polished`/restrained
  video, not a chaotic one.
- Exact SFX choice: Hyperframes should pick final filenames/timestamps/volumes
  (target volume range 0.4-0.55 for SFX given the restrained tone — quieter than
  the standard 0.55-0.85) based on the implemented animation.
- Audio files: music and the two starter SFX are already copied into
  `composition/assets/`; add any further chosen SFX there too.

## Hyperframes Instructions
Load the composition-building Hyperframes domain skills — `hyperframes-core`
(composition contract + `data-*` timing), `hyperframes-animation` (motion),
`hyperframes-creative` (design spec, beats, audio-reactive), `hyperframes-keyframes`
(seek-safe keyframes), and `hyperframes-cli` (lint/check/render). `/brag` is its own
workflow: do not enter the `hyperframes` entry-point intent interview and do not
route into its generic promo / launch-video workflow. Prefer native Hyperframes
conventions over anything in `/brag`.

Requirements:
- Show at least one real UI, copy, or visual element from the source project (the
  canvas app structure and the provenance/status vocabulary satisfy this).
- Keep all text readable in the final render — respect the reading-time floor from
  `hyperframes-animation`/`step-2-plan.md` (short label ~0.8s settled, full
  sentence ~0.3s/word, hook line gets the most).
- Keep the video within 15-25 seconds (target 20s).
- Include the planned music/SFX layer — audio was not disabled and silence was not
  chosen as the concept; restraint was chosen instead, which still means a low
  music bed plus a very small number of quiet SFX, not no audio.
- Treat `/brag` audio notes as guidance, not a fixed cue sheet. Choose exact SFX
  files/timestamps after the visual animation exists.
- Treat music cue metadata as optional timing hints; ignore cues that hurt
  readability, scene pacing, or the product story. Use at most 1-2 strong-cue locks
  given how restrained this tone is.
- Honor the planned music treatment: fade-in under the hook, steady low bed through
  the middle, a very slight (not dramatic) lift under the punchline, fade to
  silence under the outro hold.
- No audio-reactive visual treatment for this video — the tone asks for stillness.
- Use local assets already staged in `composition/assets/`.
- Run `hyperframes check` before render — it is brag's single gate.
