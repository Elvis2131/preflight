import { describe, it, expect } from "vitest";
import { MODES, capabilities } from "./modes";

describe("workspace modes (PC-104)", () => {
  it("has exactly the five modes, in order", () => {
    expect(MODES.map((m) => m.id)).toEqual(["design", "simulate", "failure_lab", "analyze", "report"]);
    expect(new Set(MODES.map((m) => m.id)).size).toBe(5);
  });

  it("only Design may edit the design or show authoring panels — every other mode is read-only", () => {
    for (const m of MODES) {
      const c = capabilities(m.id);
      expect(c.canEditDesign, m.id).toBe(m.id === "design");
      expect(c.showsAuthoringPanels, m.id).toBe(m.id === "design");
    }
  });

  it("only Failure Lab injects faults; only Simulate and Failure Lab run a baseline and show journey flow", () => {
    for (const m of MODES) {
      const c = capabilities(m.id);
      expect(c.canInjectFaults, m.id).toBe(m.id === "failure_lab");
      expect(c.canRunBaseline, m.id).toBe(m.id === "simulate" || m.id === "failure_lab");
      expect(c.showsJourneyPanel, m.id).toBe(m.id === "simulate" || m.id === "failure_lab");
    }
  });

  it("Analyze and Report are not canvas modes at all", () => {
    expect(capabilities("analyze").showsCanvas).toBe(false);
    expect(capabilities("report").showsCanvas).toBe(false);
    expect(capabilities("design").showsCanvas).toBe(true);
  });
});
