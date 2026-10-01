import type { Node } from "@xyflow/react";
import type { CanvasNodeData, CanvasSecurityGroupRule } from "./types";
import type { Fault, FaultSGRule } from "./api";

// faultBuilder.ts (PC-131): turns the architect's picks in the Failure Lab ("this
// component, this kind of fault") into the declared Fault the server simulates. It only
// NAMES faults — which kinds apply to which components is a menu, not a verdict; whether
// the fault breaks anything is the server's answer. Faults are always declared precisely
// (the Card: no "random misconfiguration" generator), so results are reproducible.

export type FaultKind = "node_loss" | "external_dependency_outage" | "nat_gateway_loss" | "region_loss" | "sg_rule_removal";

export const FAULT_KINDS: ReadonlyArray<{ id: FaultKind; label: string }> = [
  { id: "node_loss", label: "Node loss" },
  { id: "external_dependency_outage", label: "External dependency outage" },
  { id: "nat_gateway_loss", label: "NAT gateway loss" },
  { id: "region_loss", label: "Region loss" },
  { id: "sg_rule_removal", label: "Security group rule removal" },
];

export interface TargetOption {
  value: string;
  label: string;
}

// targetsFor lists what the architect may pick for a kind, from what is actually on the
// canvas (or declared in the workload, for a region). An empty list means the design has
// nothing that kind of fault could apply to.
export function targetsFor(kind: FaultKind, nodes: Node<CanvasNodeData>[], regions: string[]): TargetOption[] {
  const opt = (n: Node<CanvasNodeData>) => ({ value: n.id, label: `${n.data.label} (${n.id})` });
  switch (kind) {
    case "node_loss":
      return nodes.filter((n) => n.data.serviceID !== "aws_vpc" && n.data.serviceID !== "aws_subnet").map(opt);
    case "external_dependency_outage":
      return nodes.filter((n) => n.data.nodeType === "external_dependency").map(opt);
    case "nat_gateway_loss":
      return nodes.filter((n) => n.data.serviceID === "aws_nat_gateway").map(opt);
    case "region_loss":
      return regions.map((r) => ({ value: r, label: r }));
    case "sg_rule_removal":
      return nodes.filter((n) => (n.data.securityGroupRules ?? []).length > 0).map(opt);
  }
}

export function ruleLabel(r: CanvasSecurityGroupRule): string {
  const ports = r.from_port === undefined ? "all ports" : r.from_port === r.to_port ? `port ${r.from_port}` : `ports ${r.from_port}-${r.to_port}`;
  const src = r.source_security_group ? `from ${r.source_security_group}` : `from ${(r.cidr_blocks ?? []).join(", ") || "anywhere"}`;
  return `${r.direction} ${r.protocol} ${ports} ${src}`;
}

// sgRuleToFault names the EXACT authored rule to remove, in core.SGRule's own (untagged)
// field names, so the server matches it against the rule it ingested — never a fuzzy match.
export function sgRuleToFault(r: CanvasSecurityGroupRule): FaultSGRule {
  return {
    Direction: r.direction,
    Protocol: r.protocol,
    FromPort: r.from_port ?? 0,
    ToPort: r.to_port ?? 0,
    ...(r.cidr_blocks && r.cidr_blocks.length > 0 ? { CIDRs: r.cidr_blocks } : {}),
    ...(r.source_security_group ? { SourceSG: r.source_security_group } : {}),
  };
}

export function buildFault(kind: FaultKind, target: string, rule?: CanvasSecurityGroupRule): Fault {
  if (kind === "sg_rule_removal") {
    if (!rule) throw new Error("a security group rule removal needs the rule to remove");
    return { type: "sg_rule_change", target, sg_rule_remove: sgRuleToFault(rule) };
  }
  return { type: kind, target };
}

export function describeFault(f: Fault): string {
  switch (f.type) {
    case "node_loss":
      return `lose ${f.target}`;
    case "external_dependency_outage":
      return `${f.target} outage`;
    case "nat_gateway_loss":
      return `lose NAT ${f.target}`;
    case "region_loss":
      return `lose region ${f.target}`;
    case "sg_rule_change":
      return `remove an SG rule on ${f.target}`;
    default:
      return `${f.type} ${f.target}`;
  }
}

// killedTargets: which node IDs the canvas should paint as the ones the architect
// targeted. Presentation only — what is severed or cascaded comes from the server.
export function killedTargets(faults: Fault[]): string[] {
  return faults
    .filter((f) => f.type === "node_loss" || f.type === "external_dependency_outage" || f.type === "nat_gateway_loss")
    .map((f) => f.target);
}
