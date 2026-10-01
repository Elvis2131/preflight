import { describe, it, expect } from "vitest";
import { buildTimeline } from "./timeline";
import type { AssessResponse } from "./api";

function prov() {
  return { kind: "derived", source: "test" };
}

// A real, hand-verified 3-version sequence: finding A goes unsatisfied -> satisfied
// (improvement), finding B is new_risk in v2 and unchanged going into v3, finding C
// only appears starting v2 (a real "didn't exist yet" case in v1, not a status).
const versions: AssessResponse[] = [
  {
    session_id: "s", version_number: 1,
    findings: [], graph: null, degraded: true, compute_duration_ms: 1,
    scorecard: { version_number: 1, entries: [
      { finding_id: "A", status: "unsatisfied", provenance: prov() },
    ] },
  },
  {
    session_id: "s", version_number: 2,
    findings: [], graph: null, degraded: true, compute_duration_ms: 1,
    scorecard: { version_number: 2, entries: [
      { finding_id: "A", status: "satisfied", provenance: prov() },
      { finding_id: "B", status: "unsatisfied", provenance: prov() },
      { finding_id: "C", status: "satisfied", provenance: prov() },
    ] },
    assurance_delta: [
      { finding_id: "A", kind: "improvement", old_status: "unsatisfied", new_status: "satisfied", provenance: prov() },
      { finding_id: "B", kind: "new_risk", new_status: "unsatisfied", provenance: prov() },
    ],
  },
  {
    session_id: "s", version_number: 3,
    findings: [], graph: null, degraded: true, compute_duration_ms: 1,
    scorecard: { version_number: 3, entries: [
      { finding_id: "A", status: "satisfied", provenance: prov() },
      { finding_id: "B", status: "unsatisfied", provenance: prov() },
      { finding_id: "C", status: "satisfied", provenance: prov() },
    ] },
    assurance_delta: [
      { finding_id: "A", kind: "unchanged", old_status: "satisfied", new_status: "satisfied", provenance: prov() },
      { finding_id: "B", kind: "unchanged", old_status: "unsatisfied", new_status: "unsatisfied", provenance: prov() },
      { finding_id: "C", kind: "unchanged", old_status: "satisfied", new_status: "satisfied", provenance: prov() },
    ],
  },
];

describe("buildTimeline", () => {
  it("produces one row per finding ID, sorted, with a cell per version", () => {
    const rows = buildTimeline(versions);
    expect(rows.map((r) => r.findingID)).toEqual(["A", "B", "C"]);
    expect(rows[0].cells).toHaveLength(3);
  });

  it("finding A: unsatisfied -> satisfied, with the real improvement delta at v2", () => {
    const a = buildTimeline(versions).find((r) => r.findingID === "A")!;
    expect(a.cells[0]).toEqual({ status: "unsatisfied", deltaKind: undefined });
    expect(a.cells[1]).toEqual({ status: "satisfied", deltaKind: "improvement" });
    expect(a.cells[2]).toEqual({ status: "satisfied", deltaKind: "unchanged" });
  });

  it("finding C never appeared in v1 — a real null, not a fabricated status", () => {
    const c = buildTimeline(versions).find((r) => r.findingID === "C")!;
    expect(c.cells[0].status).toBeNull();
    expect(c.cells[0].deltaKind).toBeUndefined();
    expect(c.cells[1].status).toBe("satisfied");
  });

  it("never invents a status or delta kind beyond what the input actually contains", () => {
    const rows = buildTimeline(versions);
    const allKinds = rows.flatMap((r) => r.cells.map((c) => c.deltaKind)).filter(Boolean);
    for (const k of allKinds) {
      expect(["improvement", "regression", "new_risk", "unchanged", "not_assessable", "resolved_risk"]).toContain(k);
    }
  });
});
