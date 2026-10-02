import { describe, it, expect } from "vitest";
import type { Node, Edge } from "@xyflow/react";
import type { CanvasNodeData, CanvasEdgeData } from "./types";
import { AUTO_EDGE_PREFIX, innermostContainer, reevaluate, withAutoContainment } from "./containment";
import { serialize } from "./serialize";

function node(id: string, x: number, y: number, serviceID?: string, w?: number, h?: number): Node<CanvasNodeData> {
  return {
    id,
    type: "golden",
    position: { x, y },
    ...(w ? { style: { width: w, height: h } } : {}),
    data: { nodeType: "network_boundary", label: id, capability: {}, serviceID },
  };
}

const vpc = node("vpc", 0, 0, "aws_vpc", 600, 400);
const subnet = node("sub", 20, 40, "aws_subnet", 260, 190);
const otherSubnet = node("sub2", 320, 40, "aws_subnet", 260, 190);

describe("innermostContainer (PC-105 geometry only)", () => {
  it("a resource dropped inside a subnet inside a VPC is contained by the SUBNET, the innermost", () => {
    const db = node("db", 60, 100);
    expect(innermostContainer(db, [vpc, subnet, otherSubnet, db])).toBe("sub");
  });

  it("a resource inside the VPC but outside every subnet is not auto-linked to the VPC", () => {
    const lb = node("lb", 100, 300);
    expect(innermostContainer(lb, [vpc, subnet, otherSubnet, lb])).toBeNull();
  });

  it("a default network ACL inside a VPC (even over a subnet) is contained by the VPC, never the subnet (PC-153)", () => {
    const dflt = node("dnacl", 60, 100, "aws_default_network_acl");
    expect(innermostContainer(dflt, [vpc, subnet, dflt])).toBe("vpc");
    const outside = node("dnacl2", 900, 900, "aws_default_network_acl");
    expect(innermostContainer(outside, [vpc, subnet, outside])).toBeNull();
  });

  it("a resource outside every container is contained by none", () => {
    const lb = node("lb", 900, 900);
    expect(innermostContainer(lb, [vpc, subnet, lb])).toBeNull();
  });

  it("a subnet is contained by a VPC, never by another subnet; a VPC by nothing", () => {
    const inner = node("inner", 40, 60, "aws_subnet", 100, 80); // geometrically inside `sub`
    expect(innermostContainer(inner, [vpc, subnet, inner])).toBe("vpc");
    expect(innermostContainer(vpc, [vpc, subnet])).toBeNull();
  });
});

describe("auto containment edges", () => {
  it("replaces only the auto edge, never a hand-drawn contained_in edge", () => {
    const hand: Edge<CanvasEdgeData> = {
      id: "hand-1", source: "alb", target: "sub2", data: { edgeType: "contained_in" },
    };
    let edges = withAutoContainment([hand], "alb", "sub");
    expect(edges.map((e) => e.id).sort()).toEqual([AUTO_EDGE_PREFIX + "alb", "hand-1"].sort());
    edges = withAutoContainment(edges, "alb", null); // moved out of every container
    expect(edges.map((e) => e.id)).toEqual(["hand-1"]);
  });

  it("moving a node re-points its auto edge, and the document carries exactly that contained_in edge", () => {
    const db = node("db", 60, 100);
    const nodes = [vpc, subnet, otherSubnet, db];
    let edges = reevaluate(nodes, [], ["db"]);
    expect(serialize(nodes, edges).edges).toEqual([
      { id: AUTO_EDGE_PREFIX + "db", type: "contained_in", from: "db", to: "sub" },
    ]);
    const moved = nodes.map((n) => (n.id === "db" ? { ...n, position: { x: 360, y: 100 } } : n));
    edges = reevaluate(moved, edges, ["db"]);
    expect(serialize(moved, edges).edges.map((e) => e.to)).toEqual(["sub2"]);
  });
});

describe("serialize: placement fields (PC-105)", () => {
  it("emits availability_zone/cidr_block only when entered, never UI-only fields", () => {
    const n = node("sub", 20, 40, "aws_subnet", 260, 190);
    n.data.availabilityZone = "eu-west-1a";
    n.data.cidrBlock = "10.0.1.0/24";
    const out = serialize([n], []).nodes[0];
    expect(out).toMatchObject({ availability_zone: "eu-west-1a", cidr_block: "10.0.1.0/24", service_id: "aws_subnet" });
    expect(Object.keys(out)).not.toContain("position");
    expect(Object.keys(out)).not.toContain("style");
    expect(Object.keys(out)).not.toContain("zIndex");

    const blank = serialize([node("sub", 0, 0, "aws_subnet", 260, 190)], []).nodes[0];
    expect(Object.keys(blank)).not.toContain("availability_zone");
    expect(Object.keys(blank)).not.toContain("cidr_block");
  });
});

describe("container size", () => {
  it("an explicit container size wins over a stale measured size", () => {
    const staleVpc = { ...vpc, measured: { width: 150, height: 60 } };
    const staleSubnet = { ...subnet, measured: { width: 150, height: 60 } };
    expect(innermostContainer(staleSubnet, [staleVpc, staleSubnet])).toBe("vpc");
  });
});

describe("serialize: routes and NACL rules (PC-138/PC-139)", () => {
  it("emits routes/nacl_rules only when authored, with the wire names, and no UI-only fields", () => {
    const rt = node("rt", 0, 0, "aws_route_table");
    rt.data.routes = [{ destination_cidr: "0.0.0.0/0", target: "igw" }];
    const nacl = node("nacl", 0, 0, "aws_network_acl");
    nacl.data.naclRules = [{ direction: "ingress", number: 100, protocol: "tcp", from_port: 443, to_port: 443, cidr_block: "0.0.0.0/0", action: "allow" }];
    const [a, b] = serialize([rt, nacl], []).nodes as unknown as Array<Record<string, unknown>>;
    expect(a.routes).toEqual([{ destination_cidr: "0.0.0.0/0", target: "igw" }]);
    expect(b.nacl_rules).toEqual([{ direction: "ingress", number: 100, protocol: "tcp", from_port: 443, to_port: 443, cidr_block: "0.0.0.0/0", action: "allow" }]);
    for (const out of [a, b]) for (const k of ["naclRules", "position", "style"]) expect(Object.keys(out)).not.toContain(k);

    const untouched = serialize([node("rt2", 0, 0, "aws_route_table"), node("n2", 0, 0, "aws_network_acl")], []).nodes as unknown as Array<Record<string, unknown>>;
    for (const out of untouched) {
      expect(Object.keys(out)).not.toContain("routes");
      expect(Object.keys(out)).not.toContain("nacl_rules");
    }
  });

  it("an emptied editor serializes as 'not authored', never an empty list", () => {
    const nacl = node("nacl", 0, 0, "aws_network_acl");
    nacl.data.naclRules = [];
    nacl.data.routes = [];
    const out = serialize([nacl], []).nodes[0] as unknown as Record<string, unknown>;
    expect(Object.keys(out)).not.toContain("nacl_rules");
    expect(Object.keys(out)).not.toContain("routes");
  });
});
