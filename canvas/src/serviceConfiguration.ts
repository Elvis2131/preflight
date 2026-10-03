import type { Edge, Node } from "@xyflow/react";
import type { NodeType, EdgeType } from "./goldenVocabulary";
import type { CanvasNodeData, CanvasEdgeData } from "./types";

export type ServiceNode = Node<CanvasNodeData>;
export type ServiceEdge = Edge<CanvasEdgeData>;

const CONFIGURATION_SERVICES = new Set([
  "aws_vpc", "aws_subnet", "aws_security_group", "aws_iam_role",
  "aws_route_table", "aws_network_acl", "aws_db_subnet_group",
  "aws_elasticache_subnet_group", "aws_internet_gateway", "aws_nat_gateway",
]);

export function isConfigurationNode(node: ServiceNode): boolean {
  return CONFIGURATION_SERVICES.has(node.data.serviceID ?? "");
}

export function visibleServiceNodes(nodes: ServiceNode[], infrastructure: boolean): ServiceNode[] {
  return infrastructure ? nodes : nodes.filter((n) => !isConfigurationNode(n) || n.data.placedOnCanvas);
}

export interface ConfigurationSpec {
  id: string;
  label: string;
  serviceID: string;
  nodeType: NodeType;
  edgeType: EdgeType;
  multiple?: boolean;
  reverse?: boolean;
}

const SPECS: Record<string, ConfigurationSpec> = {
  security_groups: { id: "security_groups", label: "Security groups", serviceID: "aws_security_group", nodeType: "network_boundary", edgeType: "depends_on", multiple: true },
  iam_role: { id: "iam_role", label: "IAM role", serviceID: "aws_iam_role", nodeType: "identity", edgeType: "authenticates_via" },
  subnets: { id: "subnets", label: "Subnets", serviceID: "aws_subnet", nodeType: "network_boundary", edgeType: "contained_in", multiple: true },
  vpc: { id: "vpc", label: "VPC", serviceID: "aws_vpc", nodeType: "network_boundary", edgeType: "contained_in" },
  db_subnet_group: { id: "db_subnet_group", label: "DB subnet group", serviceID: "aws_db_subnet_group", nodeType: "network_boundary", edgeType: "contained_in" },
  cache_subnet_group: { id: "cache_subnet_group", label: "Cache subnet group", serviceID: "aws_elasticache_subnet_group", nodeType: "network_boundary", edgeType: "contained_in" },
  route_table: { id: "route_table", label: "Route table", serviceID: "aws_route_table", nodeType: "network_boundary", edgeType: "depends_on" },
  network_acl: { id: "network_acl", label: "Network ACL", serviceID: "aws_network_acl", nodeType: "network_boundary", edgeType: "depends_on" },
  internet_gateway: { id: "internet_gateway", label: "Internet gateway", serviceID: "aws_internet_gateway", nodeType: "network_boundary", edgeType: "contained_in", reverse: true },
  nat_gateway: { id: "nat_gateway", label: "NAT gateway", serviceID: "aws_nat_gateway", nodeType: "network_boundary", edgeType: "contained_in", reverse: true },
};

export function configurationSpecs(node: ServiceNode): ConfigurationSpec[] {
  const service = node.data.serviceID;
  if (service === "aws_vpc") return [SPECS.internet_gateway];
  if (service === "aws_default_network_acl") return [SPECS.vpc];
  if (service === "aws_subnet") return [SPECS.vpc, SPECS.route_table, SPECS.network_acl, SPECS.nat_gateway];
  if (service === "aws_db_subnet_group" || service === "aws_elasticache_subnet_group") return [SPECS.subnets];
  if (service === "aws_nat_gateway") return [SPECS.subnets];
  if (isConfigurationNode(node)) return [];
  if (["compute", "container_workload", "managed_database", "cache", "load_balancer"].includes(node.data.nodeType)) {
    const network = service === "aws_db_instance" ? SPECS.db_subnet_group
      : service === "aws_elasticache_replication_group" ? SPECS.cache_subnet_group : SPECS.subnets;
    const role = ["compute", "container_workload"].includes(node.data.nodeType) ? [SPECS.iam_role] : [];
    return [SPECS.security_groups, ...role, network];
  }
  return [];
}

export function attachedResources(ownerID: string, spec: ConfigurationSpec, nodes: ServiceNode[], edges: ServiceEdge[]): ServiceNode[] {
  const ids = new Set(edges.filter((e) => e.data?.edgeType === spec.edgeType && (spec.reverse ? e.target : e.source) === ownerID)
    .map((e) => spec.reverse ? e.source : e.target));
  return nodes.filter((n) => ids.has(n.id) && n.data.serviceID === spec.serviceID);
}

// Settings author ordinary graph relationships. No resource is duplicated or
// deleted when it is shared, and unrelated dependency edges remain untouched.
export function replaceAttachment(ownerID: string, spec: ConfigurationSpec, targetIDs: string[], nodes: ServiceNode[], edges: ServiceEdge[]): ServiceEdge[] {
  const validIDs = new Set(nodes.filter((n) => n.data.serviceID === spec.serviceID && n.id !== ownerID).map((n) => n.id));
  const requested = [...new Set(targetIDs)].filter((id) => validIDs.has(id));
  const selected = spec.multiple ? requested : requested.slice(0, 1);
  const old = attachedResources(ownerID, spec, nodes, edges);
  const oldIDs = new Set(old.map((n) => n.id));
  const remaining = edges.filter((e) => !(e.data?.edgeType === spec.edgeType && (spec.reverse ? e.target : e.source) === ownerID && oldIDs.has(spec.reverse ? e.source : e.target)));
  return remaining.concat(selected.map((id) => {
    const source = spec.reverse ? id : ownerID;
    const target = spec.reverse ? ownerID : id;
    const previous = edges.find((e) => e.source === source && e.target === target && e.data?.edgeType === spec.edgeType);
    return previous ?? { id: `setting:${spec.id}:${source}:${target}`, source, target, type: "default", data: { edgeType: spec.edgeType } };
  }));
}

export function configurationSummary(node: ServiceNode, nodes: ServiceNode[], edges: ServiceEdge[]): string[] {
  const groups = attachedResources(node.id, SPECS.security_groups, nodes, edges);
  const roles = attachedResources(node.id, SPECS.iam_role, nodes, edges);
  const parents = new Map<string, string[]>();
  for (const edge of edges) if (edge.data?.edgeType === "contained_in") parents.set(edge.source, [...(parents.get(edge.source) ?? []), edge.target]);
  const byID = new Map(nodes.map((n) => [n.id, n]));
  const visited = new Set<string>();
  const subnets = new Set<string>();
  const visit = (id: string) => {
    if (visited.has(id)) return;
    visited.add(id);
    if (byID.get(id)?.data.serviceID === "aws_subnet") subnets.add(id);
    for (const parent of parents.get(id) ?? []) visit(parent);
  };
  visit(node.id);
  return [
    ...(subnets.size ? [`${subnets.size} subnet${subnets.size === 1 ? "" : "s"}`] : []),
    ...(groups.length ? [`${groups.length} security group${groups.length === 1 ? "" : "s"}`] : []),
    ...(roles.length ? ["IAM role"] : []),
  ];
}

// A deterministic layout for the visible service flow. Network settings keep
// their full layout separately; neither set of positions enters the API document.
export function arrangeServiceView(nodes: ServiceNode[], edges: ServiceEdge[]): ServiceNode[] {
  const primary = nodes.filter((n) => !isConfigurationNode(n));
  const ids = new Set(primary.map((n) => n.id));
  const flow = edges.filter((e) => ids.has(e.source) && ids.has(e.target) && e.data?.edgeType !== "contained_in" && e.data?.edgeType !== "authenticates_via" && e.source !== e.target);
  const degrees = new Map(primary.map((n) => [n.id, flow.filter((e) => e.target === n.id).length]));
  const ranks = new Map(primary.map((n) => [n.id, 0]));
  const queue = primary.filter((n) => degrees.get(n.id) === 0).map((n) => n.id).sort();
  while (queue.length) {
    const id = queue.shift()!;
    for (const edge of flow.filter((e) => e.source === id)) {
      ranks.set(edge.target, Math.max(ranks.get(edge.target)!, ranks.get(id)! + 1));
      degrees.set(edge.target, degrees.get(edge.target)! - 1);
      if (degrees.get(edge.target) === 0) queue.push(edge.target);
    }
  }
  const rows = new Map<number, ServiceNode[]>();
  for (const n of primary) rows.set(ranks.get(n.id)!, [...(rows.get(ranks.get(n.id)!) ?? []), n]);
  const positions = new Map<string, { x: number; y: number }>();
  for (const [rank, row] of rows) row.sort((a, b) => a.id.localeCompare(b.id)).forEach((n, i) => positions.set(n.id, { x: (i - (row.length - 1) / 2) * 320, y: rank * 190 }));
  return nodes.map((n) => positions.has(n.id) ? {
    ...n, position: positions.get(n.id)!,
    data: { ...n.data, infrastructurePosition: n.data.infrastructurePosition ?? n.position },
  } : n);
}

export function changeCanvasView(nodes: ServiceNode[], infrastructure: boolean): ServiceNode[] {
  return nodes.map((n) => ({
    ...n,
    position: (infrastructure ? n.data.infrastructurePosition : n.data.servicePosition) ?? n.position,
    data: { ...n.data, ...(infrastructure ? { servicePosition: n.position } : { infrastructurePosition: n.position }) },
  }));
}
