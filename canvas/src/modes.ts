// modes.ts (PC-104): the workspace's five modes and what each one is allowed to do.
//
// The workspace holds UI state only — no mode computes a verdict, fault result, cost, or
// flow in the browser; every mode calls the server. Authoring is Design's alone: the
// other four modes read what the server computed, so a verdict can never be "fixed" by
// editing the picture (CLAUDE.md §3's narrowed canvas rule). That is encoded here as data
// so it is testable, not left to the discipline of whoever edits App.tsx next.

export type Mode = "design" | "simulate" | "failure_lab" | "analyze" | "report";

export const MODES: ReadonlyArray<{ id: Mode; label: string }> = [
  { id: "design", label: "Design" },
  { id: "simulate", label: "Simulate" },
  { id: "failure_lab", label: "Failure Lab" },
  { id: "analyze", label: "Analyze" },
  { id: "report", label: "Report" },
];

export interface ModeCapabilities {
  // The canvas (React Flow) is the main surface in this mode.
  showsCanvas: boolean;
  // May the architect change the design here: drop, move, connect, delete, edit nodes.
  canEditDesign: boolean;
  // Palette, inspector, NFR (workload) form, template picker.
  showsAuthoringPanels: boolean;
  // Run the no-fault baseline and read traffic flow / utilisation / latency.
  canRunBaseline: boolean;
  // Inject a fault.
  canInjectFaults: boolean;
  // The journey flow panel.
  showsJourneyPanel: boolean;
}

const NONE: ModeCapabilities = {
  showsCanvas: false,
  canEditDesign: false,
  showsAuthoringPanels: false,
  canRunBaseline: false,
  canInjectFaults: false,
  showsJourneyPanel: false,
};

export function capabilities(mode: Mode): ModeCapabilities {
  switch (mode) {
    case "design":
      return { ...NONE, showsCanvas: true, canEditDesign: true, showsAuthoringPanels: true };
    case "simulate":
      return { ...NONE, showsCanvas: true, canRunBaseline: true, showsJourneyPanel: true };
    case "failure_lab":
      return { ...NONE, showsCanvas: true, canInjectFaults: true, canRunBaseline: true, showsJourneyPanel: true };
    case "analyze":
    case "report":
      return NONE;
  }
}
