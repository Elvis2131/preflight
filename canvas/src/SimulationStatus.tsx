import type { JourneyBaseline, SimulateResponse } from "./api";

interface Props {
  result: SimulateResponse;
  runKind: "baseline" | "fault";
  labelForNode: (id: string) => string;
  onSetCapacity: () => void;
  animationStep: { index: number; total: number } | null;
  // Display name for a declared journey id; falls back to the id. Optional.
  journeyLabel?: (id: string) => string;
}

// PC-161: the first thing the architect sees is whether traffic flowed BEFORE the fault. Every word comes
// from the server's own baseline answer; nothing here decides whether a journey flows.
function baselineLine(j: JourneyBaseline, name: string, labelForNode: (id: string) => string): { text: string; tone: "ok" | "warn" | "bad" | "unknown" } {
  switch (j.status) {
    case "flows_before_and_after": return { text: `${name}: traffic flowed before the fault and still flows.`, tone: "ok" };
    case "already_blocked": return { text: `${name}: never carried traffic, even before the fault. Stopped at ${labelForNode(j.baseline_blocked_at ?? "")}: ${j.baseline_reason ?? ""}`, tone: "warn" };
    case "broken_by_fault": return { text: `${name}: flowed before the fault, and the fault breaks it at ${labelForNode(j.after_fault_blocked_at ?? "")}.`, tone: "bad" };
    case "degraded_by_fault": return { text: `${name}: flowed before the fault; now only its fallback path works${j.degraded_via ? ` (${j.degraded_via})` : ""}.`, tone: "warn" };
    default: return { text: `${name}: could not be checked before the fault. ${j.baseline_reason ?? ""}`, tone: "unknown" };
  }
}

// Labels for the engine's existing outcomes, never a new assessment.
export function SimulationStatus({ result, runKind, labelForNode, onSetCapacity, animationStep, journeyLabel = (id) => id }: Props) {
  const verdictLabels: Record<string, string> = {
    unaffected: "No structural impact detected",
    degraded: "Some paths disrupted",
    total_outage: "Entry points or all stateful paths lost",
  };
  const baseline = result.baseline;
  const baselineBlocked = baseline?.summary.state === "assessed" && baseline.summary.value === "some_declared_journeys_blocked";
  const verdict = result.verdict.state === "assessed"
    ? verdictLabels[String(result.verdict.value)] ?? `Structural result: ${String(result.verdict.value)}`
    : baselineBlocked ? "No survival verdict: the design was already broken before the fault" : "Structural impact could not be assessed";
  const baselineSummary = !baseline ? null
    : baseline.summary.state === "not_assessable" ? `Traffic flow before the fault was not fully checked: ${baseline.summary.reason ?? ""}`
      : baseline.summary.value === "all_declared_journeys_flow" ? "Traffic flows through every declared journey before the fault."
        : "Some declared journeys never carried traffic, so a survival result would mean nothing for them.";
  const baselineTone = !baseline || baseline.summary.state === "not_assessable" ? "unknown" : baseline.summary.value === "all_declared_journeys_flow" ? "ok" : "warn";
  const missingCapacity = result.capacity.state === "not_assessable" && result.capacity.reason?.startsWith('capacity_unknown: "app_node_rps"');
  const names = (ids: string[] | null) => ids?.length ? ids.map(labelForNode).join(", ") : "None";
  return <section className="status-strip simulation-status" data-testid="simulation-status" aria-label="Simulation result">
    {baseline && <div className="simulation-baseline" data-testid="baseline-summary" data-tone={baselineTone} data-state={baseline.summary.state}>
      <strong>Does traffic flow before the fault?</strong>
      <span>{baselineSummary}</span>
      {baseline.journeys.length > 0 && <ul>{baseline.journeys.map((j) => {
        const line = baselineLine(j, journeyLabel(j.journey_id), labelForNode);
        return <li key={j.journey_id} data-testid={`baseline-journey-${j.journey_id}`} data-status={j.status} data-tone={line.tone}>{line.text}</li>;
      })}</ul>}
    </div>}
    <div className="simulation-status-heading"><strong>{verdict}</strong><span>{runKind === "baseline" ? "Baseline · no fault injected" : "Fault simulation"}</span></div>
    <div className="simulation-impact"><span>Disconnected destinations: {names(result.severed_paths)}</span><span>Failure cascade: {names(result.cascade)}</span></div>
    <p className="simulation-capacity">{result.capacity.state === "assessed"
      ? `Surviving application capacity: ${String(result.capacity.value)} requests/second.`
      : missingCapacity
        ? "Application capacity is not set. Add tested requests per second in Workload → Capacity."
        : "Capacity could not be assessed. See technical details for the reason."}
      {missingCapacity && <button onClick={onSetCapacity}>Set capacity</button>}
    </p>
    <p className="simulation-status-note">Structural impact and traffic checks are separate. Review the journey results for blocked or unassessed hops.</p>
    {animationStep && <span>Showing failure step {animationStep.index} of {animationStep.total}</span>}
    <details className="simulation-diagnostics"><summary>Technical details</summary><code>{JSON.stringify({ baseline: result.baseline, verdict: result.verdict, severed_paths: result.severed_paths, cascade: result.cascade, capacity: result.capacity }, null, 2)}</code></details>
  </section>;
}
