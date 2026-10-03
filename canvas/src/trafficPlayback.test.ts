import { describe, expect, it } from "vitest";
import type { JourneyFlowResult, JourneyHopFlow } from "./api";
import { dnsLookupForJourney, trafficHopKey, trafficSteps, trafficStepCanPlay, trafficStepStopped } from "./trafficPlayback";
import type { CanvasNode } from "./types";
import type { DeclaredJourney } from "./workloadTypes";

const hop = (From: string, To: string, GroupIndex: number, Allowed = true): JourneyHopFlow => ({ From, To, GroupIndex, Allowed, Reason: Allowed ? "Allowed" : "not_assessable: missing service model" });
const flow = (Hops: JourneyHopFlow[]): JourneyFlowResult => ({ JourneyID: "test", Flows: false, Hops, ReachedByGroup: [], BlockedAt: "app-b", BlockedReason: "missing model" });

describe("traffic replay projects only returned hop results", () => {
  it("keeps parallel successes and rejected branches in the same visual step", () => {
    const result = flow([hop("internet", "lb", 1), hop("lb", "app-a", 2), hop("lb", "app-b", 2, false), hop("app-a", "db", 3)]);
    const snapshot = JSON.stringify(result);
    const steps = trafficSteps(result);
    expect(steps.map((s) => s.hops.length)).toEqual([1, 2, 1]);
    expect(steps[1].hops[1].Allowed).toBe(false);
    expect(steps.flatMap((s) => s.hops)).toEqual(result.Hops);
    expect(JSON.stringify(result)).toBe(snapshot);
  });
  it("keeps returned group order and never invents downstream hops", () => {
    const result = flow([hop("internet", "lb", 1), hop("lb", "app", 2, false), hop("internet", "fallback", 1)]);
    expect(trafficSteps(result).map((s) => s.hops[0].To)).toEqual(["lb", "app", "fallback"]);
    expect(trafficSteps(null)).toEqual([]);
  });
  it("retains the final backend gate when authorization rejects an otherwise allowed network hop", () => {
    const network = hop("app", "db", 1);
    const authorization = { ...hop("app", "db", 1, false), Reason: "Blocked at iam_authorization: explicit deny" };
    expect(trafficSteps(flow([network, authorization]))).toEqual([{ hops: [authorization] }]);
    expect(network.Allowed).toBe(true);
  });
  it("identifies directional pairs without collisions from separator characters in IDs", () => {
    expect(trafficHopKey("a|b", "c")).not.toBe(trafficHopKey("a", "b|c"));
    expect(trafficHopKey("a", "b")).not.toBe(trafficHopKey("b", "a"));
  });
  it("shows declared DNS first without inventing a DNS assessment or proxy hop", () => {
    const savedNodes: CanvasNode[] = [{ id: "dns", type: "dns", label: "Route 53", service_id: "aws_route53_record", capability: { design_dns_client: "internet", design_alias: "lb" } }];
    const journey: DeclaredJourney = { id: "web", name: "Web", path: ["internet", "lb", "app"], protocol: "tcp", port: 443, criticality: "tier2" };
    const dns = dnsLookupForJourney(savedNodes, journey);
    const result = flow([hop("internet", "lb", 1), hop("lb", "app", 2, false)]);
    const before = JSON.stringify(result);
    const steps = trafficSteps(result, dns);
    expect(steps[0]).toEqual({ hops: [], dns: { From: "internet", To: "dns", ResolvesTo: "lb" } });
    expect(trafficStepCanPlay(steps[0])).toBe(true);
    expect(trafficStepStopped(steps[0])).toBe(false);
    expect(steps.slice(1)).toEqual(trafficSteps(result));
    expect(trafficStepStopped(steps[2])).toBe(true);
    expect(JSON.stringify(result)).toBe(before);
    expect(result.Hops.some((h) => h.From === "dns" || h.To === "dns")).toBe(false);
    expect(dnsLookupForJourney(savedNodes, { ...journey, path: ["lb", "app"] })).toBeNull();
    expect(dnsLookupForJourney(savedNodes, { ...journey, path: ["internet", "other-lb"] })).toBeNull();
    expect(dnsLookupForJourney([{ ...savedNodes[0], capability: { design_alias: "lb" } }], journey)).toBeNull();
    expect(trafficSteps(null, dns)).toEqual([]);
    expect(trafficSteps(flow([hop("lb", "app", 1)]), dns)).toHaveLength(1);
  });
});
