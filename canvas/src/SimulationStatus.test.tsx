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

// PC-161: baseline validity comes first; every word is the server's answer, never a client verdict.
const flow = { state: "assessed" as const, value: "all_declared_journeys_flow" };
const blocked = { state: "assessed" as const, value: "some_declared_journeys_blocked" };
describe("baseline validity (PC-161)", () => {
  const text = (r: SimulateResponse) => visibleText(render(r, "fault"));
  it("says traffic flows before the fault and labels a journey the fault breaks", () => {
    const t = text({ ...result, baseline: { summary: flow, journeys: [
      { journey_id: "web", status: "flows_before_and_after" },
      { journey_id: "data", status: "broken_by_fault", after_fault_blocked_at: "db" },
    ] } });
    expect(t).toContain("Does traffic flow before the fault?");
    expect(t).toContain("Traffic flows through every declared journey before the fault.");
    expect(t).toContain("web: traffic flowed before the fault and still flows.");
    expect(t).toContain("data: flowed before the fault, and the fault breaks it at Private database.");
  });
  it("names a journey that never worked and withholds the survival verdict", () => {
    const t = text({ ...result, verdict: { state: "not_assessable", reason: "journey was already blocked" },
      baseline: { summary: blocked, journeys: [{ journey_id: "api", status: "already_blocked", baseline_blocked_at: "app", baseline_reason: "no rule matched" }] } });
    expect(t).toContain("Some declared journeys never carried traffic");
    expect(t).toContain("api: never carried traffic, even before the fault. Stopped at Application: no rule matched");
    expect(t).toContain("No survival verdict: the design was already broken before the fault");
    expect(t).not.toContain("No structural impact detected");
  });
  it("says plainly when nothing was checked, and when a journey could not be checked", () => {
    const none = text({ ...result, baseline: { summary: { state: "not_assessable", reason: "no journey is declared, so whether traffic flows through this design was not checked" }, journeys: [] } });
    expect(none).toContain("Traffic flow before the fault was not fully checked: no journey is declared");
    const unsure = text({ ...result, baseline: { summary: { state: "not_assessable", reason: "undecided" }, journeys: [{ journey_id: "settle", status: "not_assessable", baseline_reason: "no resolvable route table" }] } });
    expect(unsure).toContain("settle: could not be checked before the fault. no resolvable route table");
  });
  it("shows nothing extra against an older server that sends no baseline", () => {
    expect(text(result)).not.toContain("Does traffic flow before the fault?");
  });
});

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
