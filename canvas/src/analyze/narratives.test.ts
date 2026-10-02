import { describe, it, expect } from "vitest";
import { narrativeReducer, initialNarrativeState, fromStored, type Narrative, type StoredNarratives } from "./narratives";

const n = (id: string): Narrative => ({ finding_id: id, narrative: "About " + id, cited_evidence: ["aws_lb.payments"], provenance: { kind: "llm_reasoned", source: "reason/x", reason: "cites aws_lb.payments" } });

describe("narrativeReducer (PC-154): display state from the server's stream, never a verdict", () => {
  it("accumulates annotations as they arrive and completes", () => {
    let s = narrativeReducer(initialNarrativeState, { type: "start" });
    expect(s.phase).toBe("streaming");
    s = narrativeReducer(s, { type: "annotation", data: n("a") });
    s = narrativeReducer(s, { type: "annotation", data: n("b") });
    s = narrativeReducer(s, { type: "done", status: "complete" });
    expect(s.phase).toBe("complete");
    expect(s.annotations.map((a) => a.finding_id)).toEqual(["a", "b"]);
  });

  it("a degraded event is a state, not an error: partial results stay visible with the reason", () => {
    let s = narrativeReducer(initialNarrativeState, { type: "start" });
    s = narrativeReducer(s, { type: "annotation", data: n("a") });
    s = narrativeReducer(s, { type: "rejected", data: { finding_id: "b", reason: "model call failed: HTTP 429", kind: "call_failed" } });
    s = narrativeReducer(s, { type: "degraded", reason: "1 of 2 findings could not be annotated" });
    s = narrativeReducer(s, { type: "done", status: "degraded" });
    expect(s.phase).toBe("degraded");
    expect(s.annotations).toHaveLength(1);
    expect(s.rejected).toHaveLength(1);
    expect(s.degradedReason).toContain("could not be annotated");
  });

  it("a degraded reason seen before done wins even if done says complete (never shown as a clean success)", () => {
    let s = narrativeReducer(initialNarrativeState, { type: "start" });
    s = narrativeReducer(s, { type: "degraded", reason: "worker unreachable" });
    s = narrativeReducer(s, { type: "done", status: "complete" });
    expect(s.phase).toBe("degraded");
  });

  it("an interrupted stream degrades with a stated reason; an interruption after done changes nothing", () => {
    let s = narrativeReducer(narrativeReducer(initialNarrativeState, { type: "start" }), { type: "interrupted" });
    expect(s.phase).toBe("degraded");
    expect(s.degradedReason).toMatch(/interrupted/);
    s = narrativeReducer(narrativeReducer(narrativeReducer(initialNarrativeState, { type: "start" }), { type: "done", status: "complete" }), { type: "interrupted" });
    expect(s.phase).toBe("complete");
  });

  it("starting again clears the previous run", () => {
    let s = narrativeReducer(narrativeReducer(initialNarrativeState, { type: "start" }), { type: "annotation", data: n("a") });
    s = narrativeReducer(s, { type: "start" });
    expect(s.annotations).toEqual([]);
  });

  it("fromStored maps a stored set, keeping a degraded set's reason", () => {
    const set: StoredNarratives = { status: "degraded", reason: "2 of 16 findings could not be annotated", model: "m", annotations: [n("a")], rejected: [], notice: "x" };
    const s = fromStored(set);
    expect(s.phase).toBe("degraded");
    expect(s.degradedReason).toContain("2 of 16");
    expect(fromStored({ ...set, status: "complete", reason: undefined }).phase).toBe("complete");
  });
});

describe("load/reset (PC-154)", () => {
  it("load replaces the state, reset clears it", () => {
    const loaded = fromStored({ status: "complete", annotations: [{ finding_id: "a", narrative: "x", cited_evidence: [], provenance: { kind: "llm_reasoned", source: "s" } }], rejected: [], notice: "n" });
    expect(narrativeReducer(initialNarrativeState, { type: "load", state: loaded }).annotations).toHaveLength(1);
    expect(narrativeReducer(loaded, { type: "reset" })).toEqual(initialNarrativeState);
  });
});
