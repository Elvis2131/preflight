import { describe, it, expect } from "vitest";
import journeyPanelSource from "./JourneyPanel.tsx?raw";
import appSource from "./App.tsx?raw";

// Structural "no client-side flow/load computation" tests — same discipline as
// ReportView.test.ts (PC-123): read the component's own source via Vite's ?raw
// import (no Node fs APIs, so `npm run build`'s stricter tsconfig.app.json — which
// lacks "node" types — never breaks on this file, a real bug PC-123 already hit
// once). PC-125/126's own Card text is explicit that flow/utilization are computed
// ONCE, server-side, and never duplicated in the canvas.
const source = journeyPanelSource + "\n" + appSource;

describe("JourneyPanel/App: no client-side flow or utilization computation", () => {
  it("never divides an offered-load figure by a capacity figure to derive a ratio", () => {
    // The real utilization ratio must only ever be READ from ComponentLoad.Utilization
    // (server-computed) — never rebuilt here as offered/capacity.
    expect(source).not.toMatch(/OfferedRPS\s*\/\s*\w*[Cc]apacity/);
    expect(source).not.toMatch(/\w*[Cc]apacity\w*\s*\/\s*\w*OfferedRPS/);
  });

  it("has no function whose name implies it computes flow or load itself", () => {
    expect(source).not.toMatch(/function\s+compute(Flow|Load|Utilization)/i);
    expect(source).not.toMatch(/const\s+compute(Flow|Load|Utilization)\s*=/i);
  });

  it("only ever reads Utilization/OfferedRPS off a server response object, never assigns into them", () => {
    // A real re-derivation would assign a freshly computed number onto these fields;
    // this app must only ever read them (l.Utilization, load.Utilization, etc.).
    expect(source).not.toMatch(/\.Utilization\s*=[^=]/);
    expect(source).not.toMatch(/\.OfferedRPS\s*=[^=]/);
  });
});
