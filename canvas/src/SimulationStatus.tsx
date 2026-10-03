import type { SimulateResponse } from "./api";

interface Props {
  result: SimulateResponse;
  runKind: "baseline" | "fault";
  labelForNode: (id: string) => string;
  onSetCapacity: () => void;
  animationStep: { index: number; total: number } | null;
}

// Labels for the engine's existing outcomes, never a new assessment.
export function SimulationStatus({ result, runKind, labelForNode, onSetCapacity, animationStep }: Props) {
  const verdictLabels: Record<string, string> = {
    unaffected: "No structural impact detected",
    degraded: "Some paths disrupted",
    total_outage: "Entry points or all stateful paths lost",
  };
  const verdict = result.verdict.state === "assessed"
    ? verdictLabels[String(result.verdict.value)] ?? `Structural result: ${String(result.verdict.value)}`
    : "Structural impact could not be assessed";
  const missingCapacity = result.capacity.state === "not_assessable" && result.capacity.reason?.startsWith('capacity_unknown: "app_node_rps"');
  const names = (ids: string[] | null) => ids?.length ? ids.map(labelForNode).join(", ") : "None";
  return <section className="status-strip simulation-status" data-testid="simulation-status" aria-label="Simulation result">
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
    <details className="simulation-diagnostics"><summary>Technical details</summary><code>{JSON.stringify({ verdict: result.verdict, severed_paths: result.severed_paths, cascade: result.cascade, capacity: result.capacity }, null, 2)}</code></details>
  </section>;
}
