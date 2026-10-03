import { describe, expect, it } from "vitest";
import { arrangeServiceView, attachedResources, changeCanvasView, configurationSpecs, configurationSummary, replaceAttachment, visibleServiceNodes, type ServiceNode, type ServiceEdge } from "./serviceConfiguration";
import { serialize } from "./serialize";

const node = (id: string, serviceID: string, nodeType: ServiceNode["data"]["nodeType"]): ServiceNode => ({ id, type: "golden", position: { x: 500, y: 500 }, data: { label: id, serviceID, nodeType, capability: {} } });
const edge = (source: string, target: string, edgeType: "depends_on" | "contained_in" | "authenticates_via"): ServiceEdge => ({ id: source + target, source, target, data: { edgeType } });
const app = node("app", "aws_instance", "compute");
const other = node("other", "aws_instance", "compute");
const sg = node("sg", "aws_security_group", "network_boundary");
const role = node("role", "aws_iam_role", "identity");
const oldRole = node("old", "aws_iam_role", "identity");
const subnet = node("subnet", "aws_subnet", "network_boundary");
const group = node("group", "aws_db_subnet_group", "network_boundary");
const db = node("db", "aws_db_instance", "managed_database");
const nodes = [app, other, sg, role, oldRole, subnet, group, db];

describe("service attributes backed by graph relationships", () => {
  it("removes a security-group assignment without deleting the shared group or another service's assignment", () => {
    const spec = configurationSpecs(app).find((s) => s.id === "security_groups")!;
    const edges = [edge("app", "sg", "depends_on"), edge("other", "sg", "depends_on"), edge("app", "db", "depends_on")];
    const next = replaceAttachment("app", spec, [], nodes, edges);
    expect(next).toEqual(edges.slice(1));
    expect(nodes).toContain(sg);
  });

  it("assigns one real IAM role, rejects incompatible resources, and preserves other dependencies", () => {
    const spec = configurationSpecs(app).find((s) => s.id === "iam_role")!;
    const edges = [edge("app", "old", "authenticates_via"), edge("app", "db", "depends_on")];
    const next = replaceAttachment("app", spec, ["sg", "role", "old"], nodes, edges);
    expect(attachedResources("app", spec, nodes, next).map((n) => n.id)).toEqual(["role"]);
    expect(next).toContain(edges[1]);
    expect(serialize(nodes, next).edges).toContainEqual(expect.objectContaining({ type: "authenticates_via", from: "app", to: "role" }));
  });

  it("resolves network placement through a subnet group without flattening the graph", () => {
    const edges = [edge("db", "group", "contained_in"), edge("group", "subnet", "contained_in"), edge("db", "sg", "depends_on")];
    expect(configurationSummary(db, nodes, edges)).toEqual(["1 subnet", "1 security group"]);
  });

  it("replaces network controls without disturbing other subnet dependencies", () => {
    const table = node("table", "aws_route_table", "network_boundary");
    const acl = node("acl", "aws_network_acl", "network_boundary");
    const subnetSpec = configurationSpecs(subnet).find((s) => s.id === "route_table")!;
    const edges = [edge("subnet", "table", "depends_on"), edge("subnet", "acl", "depends_on")];
    expect(replaceAttachment("subnet", subnetSpec, [], [...nodes, table, acl], edges)).toEqual([edges[1]]);
  });

  it("attaches gateways in the correct direction while preserving the shared network", () => {
    const vpc = node("vpc", "aws_vpc", "network_boundary");
    const gateway = node("gateway", "aws_internet_gateway", "network_boundary");
    const spec = configurationSpecs(vpc)[0];
    const next = replaceAttachment(vpc.id, spec, [gateway.id], [vpc, gateway], []);
    expect(serialize([vpc, gateway], next).edges).toEqual([{ id: "setting:internet_gateway:gateway:vpc", from: "gateway", to: "vpc", type: "contained_in" }]);
    expect(attachedResources(vpc.id, spec, [vpc, gateway], next)).toEqual([gateway]);
  });

  it("retains the entire backend document while switching between clean and infrastructure views", () => {
    const edges = [edge("app", "db", "depends_on"), edge("app", "sg", "depends_on"), edge("app", "role", "authenticates_via")];
    const original = serialize(nodes, edges);
    const clean = arrangeServiceView(nodes, edges);
    expect(visibleServiceNodes(clean, false).map((n) => n.id)).toEqual(["app", "other", "db"]);
    expect(serialize(clean, edges)).toEqual(original);
    const full = changeCanvasView(clean, true);
    expect(full.map((n) => n.position)).toEqual(nodes.map((n) => n.position));
    expect(serialize(full, edges)).toEqual(original);
    expect(changeCanvasView(full, false).map((n) => n.position)).toEqual(clean.map((n) => n.position));
  });

  it("keeps explicitly dropped configuration resources visible and handles cyclic flows without hanging", () => {
    expect(visibleServiceNodes([{ ...sg, data: { ...sg.data, placedOnCanvas: true } }], false)).toHaveLength(1);
    const cyc = arrangeServiceView([app, other], [edge("app", "other", "depends_on"), edge("other", "app", "depends_on")]);
    expect(cyc[0].position).not.toEqual(cyc[1].position);
  });
});
