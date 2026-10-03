import type { JourneyFlowResult, JourneyHopFlow } from "./api";
import type { CanvasNode } from "./types";
import type { DeclaredJourney } from "./workloadTypes";

export interface DnsLookup {
  From: "internet";
  To: string;
  ResolvesTo: string;
}

// An explicitly saved DNS relationship is architecture intent, not a returned
// Allowed result. Match only its declared internet journey and alias target.
export function dnsLookupForJourney(nodes: CanvasNode[], journey?: DeclaredJourney): DnsLookup | null {
  if (journey?.path[0] !== "internet") return null;
  const dns = nodes.find((n) => n.service_id === "aws_route53_record" && n.capability.design_dns_client === "internet"
    && n.capability.design_alias === journey.path[1]);
  return dns ? { From: "internet", To: dns.id, ResolvesTo: dns.capability.design_alias } : null;
}

export interface TrafficStep {
  hops: JourneyHopFlow[];
  dns?: DnsLookup;
}

export const trafficStepCanPlay = (step?: TrafficStep | null) => !!step && (!!step.dns || step.hops.some((h) => h.Allowed));
export const trafficStepStopped = (step?: TrafficStep | null) => !!step && !step.dns && step.hops.length > 0 && step.hops.every((h) => !h.Allowed);

// Presentation groups only. Preserve the server's order and rejected branches;
// consecutive members of one GroupIndex are simultaneous, not extra serial hops.
// The server can return an IAM gate after a network gate for the SAME pair; its
// last returned decision must win, so a later denial never gets an allowed packet.
export function trafficSteps(flow: JourneyFlowResult | null, dns?: DnsLookup | null): TrafficStep[] {
  const steps: TrafficStep[] = [];
  if (dns && flow?.Hops.some((h) => h.From === dns.From && h.To === dns.ResolvesTo)) steps.push({ hops: [], dns });
  for (const hop of flow?.Hops ?? []) {
    const previous = steps.at(-1);
    if (previous?.hops[0]?.GroupIndex === hop.GroupIndex) {
      const repeated = previous.hops.findIndex((h) => h.From === hop.From && h.To === hop.To);
      if (repeated >= 0) previous.hops[repeated] = hop;
      else previous.hops.push(hop);
    }
    else steps.push({ hops: [hop] });
  }
  return steps;
}

export function trafficHopKey(from: string, to: string): string {
  return JSON.stringify([from, to]);
}
