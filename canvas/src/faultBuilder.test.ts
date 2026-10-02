import { describe, it, expect } from "vitest";
import type { Edge, Node } from "@xyflow/react";
import type { CanvasEdgeData, CanvasNodeData } from "./types";
import { targetsFor, registeredTargets, buildFault, sgRuleToFault, killedTargets, describeFault, FAULT_KINDS } from "./faultBuilder";

function n(id: string, nodeType: CanvasNodeData["nodeType"], extra: Partial<CanvasNodeData> = {}): Node<CanvasNodeData> {
  return { id, type: "golden", position: { x: 0, y: 0 }, data: { nodeType, label: id.toUpperCase(), capability: {}, ...extra } };
}

const nodes = [
  n("vpc", "network_boundary", { serviceID: "aws_vpc" }),
  n("sub", "network_boundary", { serviceID: "aws_subnet" }),
  n("rail", "external_dependency"),
  n("nat1", "network_boundary", { serviceID: "aws_nat_gateway" }),
  n("sg", "network_boundary", { securityGroupRules: [{ direction: "ingress", protocol: "tcp", from_port: 5432, to_port: 5432, cidr_blocks: ["10.0.1.0/24"] }] }),
  n("db", "managed_database"),
];

describe("faultBuilder (PC-131): names faults, never decides what they break", () => {
  it("offers only what a kind can apply to, from the actual design", () => {
    expect(targetsFor("external_dependency_outage", nodes, []).map((t) => t.value)).toEqual(["rail"]);
    expect(targetsFor("nat_gateway_loss", nodes, []).map((t) => t.value)).toEqual(["nat1"]);
    expect(targetsFor("sg_rule_removal", nodes, []).map((t) => t.value)).toEqual(["sg"]);
    expect(targetsFor("region_loss", nodes, ["eu-west-1"]).map((t) => t.value)).toEqual(["eu-west-1"]);
    // containers are not "lost" as a node; everything else is
    expect(targetsFor("node_loss", nodes, []).map((t) => t.value)).toEqual(["rail", "nat1", "sg", "db"]);
  });

  it("an empty design offers nothing, so nothing can be added", () => {
    for (const k of FAULT_KINDS) expect(targetsFor(k.id, [], [])).toEqual([]);
  });

  it("builds the exact declared fault the server expects", () => {
    expect(buildFault("node_loss", "db")).toEqual({ type: "node_loss", target: "db" });
    expect(buildFault("external_dependency_outage", "rail")).toEqual({ type: "external_dependency_outage", target: "rail" });
    const rule = nodes[4].data.securityGroupRules![0];
    expect(buildFault("sg_rule_removal", "sg", rule)).toEqual({
      type: "sg_rule_change",
      target: "sg",
      // core.SGRule has no json tags, so its own field names go on the wire
      sg_rule_remove: { Direction: "ingress", Protocol: "tcp", FromPort: 5432, ToPort: 5432, CIDRs: ["10.0.1.0/24"] },
    });
  });

  it("an SG rule removal without the rule is refused, never guessed", () => {
    expect(() => buildFault("sg_rule_removal", "sg")).toThrow();
  });

  it("omits an empty CIDR/SG source rather than sending a blank one", () => {
    expect(sgRuleToFault({ direction: "egress", protocol: "-1" })).toEqual({ Direction: "egress", Protocol: "-1", FromPort: 0, ToPort: 0 });
  });

  it("paints as 'killed' only the nodes a fault targets as lost", () => {
    const faults = [buildFault("node_loss", "db"), buildFault("external_dependency_outage", "rail"), buildFault("region_loss", "eu-west-1"), buildFault("sg_rule_removal", "sg", nodes[4].data.securityGroupRules![0])];
    expect(killedTargets(faults)).toEqual(["db", "rail"]);
  });

  it("describes a fault in words", () => {
    expect(describeFault({ type: "node_loss", target: "db" })).toBe("lose db");
    expect(describeFault({ type: "mystery", target: "x" })).toBe("mystery x");
  });
});

describe("target deregistration (PC-130)", () => {
  const lbNodes = [n("lb", "load_balancer"), n("lb2", "load_balancer"), n("app", "container_workload"), n("db", "managed_database")];
  const e = (id: string, source: string, target: string, edgeType: CanvasEdgeData["edgeType"]): Edge<CanvasEdgeData> => ({ id, source, target, data: { edgeType } as CanvasEdgeData });
  const edges = [e("1", "lb", "app", "routes_to"), e("2", "app", "db", "reads/writes")];

  it("offers only a load balancer that has drawn targets", () => {
    expect(targetsFor("target_deregistration", lbNodes, [], edges).map((t) => t.value)).toEqual(["lb"]);
    expect(targetsFor("target_deregistration", lbNodes, [], [])).toEqual([]);
  });

  it("lists a load balancer's own routes_to targets, nothing else", () => {
    expect(registeredTargets("lb", lbNodes, edges).map((t) => t.value)).toEqual(["app"]);
    expect(registeredTargets("lb2", lbNodes, edges)).toEqual([]);
  });

  it("builds the declared fault, refusing to guess the target", () => {
    expect(buildFault("target_deregistration", "lb", undefined, "app")).toEqual({ type: "target_deregistration", target: "lb", deregister_target: "app" });
    expect(() => buildFault("target_deregistration", "lb")).toThrow();
    expect(describeFault({ type: "target_deregistration", target: "lb", deregister_target: "app" })).toBe("deregister app from lb");
  });
});
