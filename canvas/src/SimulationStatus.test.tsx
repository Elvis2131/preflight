import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { SimulateResponse } from "./api";
import { SimulationStatus } from "./SimulationStatus";

const result: SimulateResponse = {
  journeys: [], flow_detail: [], load: [], severed_paths: [], cascade: [],
  verdict: { state: "assessed", value: "unaffected" },
  capacity: { state: "not_assessable", reason: 'capacity_unknown: "app_node_rps" was not declared in workload.yaml — CLAUDE.md §9' },
};
const render = (r = result, runKind: "baseline" | "fault" = "baseline") => renderToStaticMarkup(<SimulationStatus result={r} runKind={runKind} labelForNode={(id) => ({ db: "Private database", app: "Application" })[id] ?? id} onSetCapacity={() => {}} animationStep={null} />);
const visibleText = (markup: string) => markup.replace(/<details[\s\S]*?<\/details>/g, "");

describe("simulation result presentation", () => {
  it("explains an unaffected baseline without implying traffic or capacity passed", () => {
    const markup = render();
    expect(visibleText(markup)).toContain("No structural impact detected");
    expect(visibleText(markup)).toContain("Baseline · no fault injected");
    expect(visibleText(markup)).toContain("Application capacity is not set");
    expect(visibleText(markup)).toContain("Structural impact and traffic checks are separate");
    expect(visibleText(markup)).not.toMatch(/CLAUDE\.md|workload\.yaml|capacity_unknown|severed_paths/);
    expect(markup).toContain("CLAUDE.md §9"); // Original evidence remains available.
  });
  it("retains unknown states rather than presenting a successful verdict", () => {
    const markup = visibleText(render({ ...result, verdict: { state: "not_assessable", reason: "Unmapped service" }, capacity: { state: "not_assessable", reason: "Unsupported assessment" } }));
    expect(markup).toContain("Structural impact could not be assessed");
    expect(markup).toContain("Capacity could not be assessed");
    expect(markup).not.toContain("Set capacity");
  });
  it("labels affected resources by name and preserves explicit zero capacity", () => {
    const markup = visibleText(render({ ...result, verdict: { state: "assessed", value: "degraded" }, capacity: { state: "assessed", value: 0 }, severed_paths: ["db"], cascade: ["app", "db"] }, "fault"));
    expect(markup).toContain("Some paths disrupted");
    expect(markup).toContain("Fault simulation");
    expect(markup).toContain("Disconnected destinations: Private database");
    expect(markup).toContain("Failure cascade: Application, Private database");
    expect(markup).toContain("0 requests/second");
    expect(markup).not.toContain("Application capacity is not set");
  });
});
