// The canonical node/edge vocabulary — mirrored deliberately, not redefined, from
// preflight/core/ir.go's own frozen core.NodeType/core.EdgeType enums (PC-7). This
// file is the ONE place that mirror happens; every other file in this package
// imports from here rather than repeating string literals, so a future drift between
// the Go enum and this list has exactly one place to fix.
//
// PC-85's own Card text lists 10 node types, omitting container_workload — read as
// an incomplete restatement in the ticket's prose, not a deliberate exclusion: EKS/
// AKS (container_workload) is one of the golden 8 node types (CLAUDE.md §14) and is
// very much real IR vocabulary. The authoritative source is core/ir.go's actual enum
// (11 values), not the ticket's own prose summary of it — verified by reading
// core/ir.go directly, not assumed from the ticket text alone.

export const NODE_TYPES = [
  "compute",
  "container_workload",
  "managed_database",
  "cache",
  "load_balancer",
  "queue/stream",
  "object_store",
  "dns",
  "network_boundary",
  "identity",
  "external_dependency",
] as const;

export type NodeType = (typeof NODE_TYPES)[number];

export const EDGE_TYPES = [
  "depends_on",
  "routes_to",
  "reads/writes",
  "authenticates_via",
  "replicates_to",
  "contained_in",
] as const;

export type EdgeType = (typeof EDGE_TYPES)[number];

export const EDGE_TYPE_LABELS: Record<EdgeType, string> = {
  depends_on: "Depends on",
  routes_to: "Routes traffic to",
  "reads/writes": "Reads / writes data",
  authenticates_via: "Authenticates through",
  replicates_to: "Replicates data to",
  contained_in: "Placed inside",
};

export const EDGE_TYPE_GUIDES: Record<EdgeType, string> = {
  depends_on: "Connect a service to another service it needs to work.",
  routes_to: "Connect the service sending traffic to the service receiving it.",
  "reads/writes": "Connect an application to the database or storage it uses.",
  authenticates_via: "Connect a service to the identity service it uses to sign in.",
  replicates_to: "Connect the source database or store to its replica.",
  contained_in: "Connect a resource to the subnet or VPC that contains it.",
};

// NODE_TYPE_LABELS: a human-readable label per node type, for the palette. Purely
// presentational — never part of the serialized CanvasDocument (see types.ts).
export const NODE_TYPE_LABELS: Record<NodeType, string> = {
  compute: "Compute",
  container_workload: "Container Workload",
  managed_database: "Managed Database",
  cache: "Cache",
  load_balancer: "Load Balancer",
  "queue/stream": "Queue / Stream",
  object_store: "Object Store",
  dns: "DNS",
  network_boundary: "Network Boundary",
  identity: "Identity",
  external_dependency: "External Dependency",
};

export const DEFAULT_EDGE_TYPE: EdgeType = "depends_on";
