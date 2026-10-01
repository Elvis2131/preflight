import type { NodeType, EdgeType } from "./goldenVocabulary";

// CanvasDocument is the ONLY shape this app ever serializes to — PC-85's own
// acceptance criterion, verbatim: "canvas state (nodes + edges + capability values)
// is serialisable to a plain JSON document with no UI-only fields leaking into it."
// No position, no selection state, no React Flow internals (viewport, dragging,
// z-index, handle IDs) ever appear here — see serialize.ts, the one function
// permitted to produce this shape from React Flow's own live state.
//
// As of PC-86, this is the MIRROR of core.CanvasDocument (preflight/core/canvas.go)
// — the sixth frozen contract, contracts/canvas.schema.json — not the reverse. A
// future change here (PC-87 adding real capability structure, for instance) means
// bumping that contract's version first, per contracts/CHANGELOG.md's own convention,
// not just editing this file.
export interface CanvasNode {
  id: string;
  type: NodeType;
  label: string;
  // capability: free-form key/value pairs, deliberately untyped at the shell stage.
  // PC-87 (the NFR form) and a later capability-form ticket own giving these real,
  // per-node-type structure and validation against workload.schema.json's own
  // semantics; this ticket is shell only (its own Card: "no backend wiring").
  capability: Record<string, string>;
  // sizing: PC-110's own addition — free-form key/value pairs entered via the Design
  // inspector, same untyped-at-the-shell-stage shape as capability above. Keys are
  // core.Sizing's own canonical field names (see sizingFields.ts) — real structure
  // lives in ingest/canvas.go's buildCanvasSizing, not here. Omitted (not an empty
  // object) when the architect has not opened the inspector for this node at all,
  // matching capability's own "absence means nothing entered" convention.
  sizing?: Record<string, string>;
  // service_id: PC-136's own addition — the real provider Terraform resource_type
  // this node represents (e.g. "aws_db_instance"), the same key providers.Registry
  // is keyed by for the Terraform ingest path. Absent (never a guessed default)
  // means the architect has not picked a specific service yet — CanvasNode.Type
  // alone ("compute") cannot distinguish EC2 from Lambda, which carry different
  // PC-107 capability levels, so a canvas node with no service_id resolves no
  // capability_level at all and every journey through it is honestly
  // not_assessable (core/trace.go's own capability gate).
  service_id?: string;
  // security_group_rules: PC-137's own addition — real, structured Security Group
  // rules authored on this node, mirroring core.CanvasSecurityGroupRule field for
  // field. A structured list, not a string map like capability/sizing, since one
  // rule is a small record (direction, protocol, ports, source). A component
  // attaches to this security group via a depends_on edge — the same edge type
  // this app already uses for every other structural relationship, no new
  // attachment mechanism. Absent (never an empty array) means the architect has
  // not authored any rule for this node yet — core/trace.go's own sg_dest_ingress
  // step reports not_assessable for a component with no security group attached
  // at all, never a guessed allow or deny.
  security_group_rules?: CanvasSecurityGroupRule[];
  // availability_zone/cidr_block: PC-105's own addition — placement facts for VPC/subnet
  // containers, mirroring core.CanvasNode field for field. A subnet's one zone (a subnet
  // cannot span zones) and a VPC's/subnet's IPv4 range. Absent means unknown, never
  // defaulted. The Region ⊃ VPC ⊃ AZ ⊃ Subnet ⊃ resource nesting travels as contained_in
  // edges plus these attributes — an AZ is an attribute of a subnet, not a node.
  availability_zone?: string;
  cidr_block?: string;
  // routes: PC-138's own addition — routes authored on a ROUTE TABLE node, mirroring
  // core.CanvasRoute. A subnet associates with the table by a depends_on edge to it.
  routes?: CanvasRoute[];
  // nacl_rules: PC-139's own addition — rules authored on a NETWORK ACL node, mirroring
  // core.CanvasNACLRule. A subnet associates by a depends_on edge to it.
  nacl_rules?: CanvasNACLRule[];
}

export interface CanvasRoute {
  destination_cidr: string;
  // target is another node's ID — an internet gateway or a NAT gateway (the only target
  // kinds the engine models; the server rejects any other with invalid_network_controls).
  target: string;
}

export interface CanvasNACLRule {
  direction: "ingress" | "egress";
  number: number;
  protocol: string;
  from_port?: number;
  to_port?: number;
  cidr_block: string;
  action: "allow" | "deny";
}

// CanvasSecurityGroupRule mirrors core.CanvasSecurityGroupRule (core/canvas.go,
// PC-137) field for field — the wire shape for one authored Security Group rule.
export interface CanvasSecurityGroupRule {
  direction: "ingress" | "egress";
  protocol: string;
  from_port?: number;
  to_port?: number;
  cidr_blocks?: string[];
  source_security_group?: string;
}

export interface CanvasEdge {
  id: string;
  type: EdgeType;
  from: string;
  to: string;
}

export interface CanvasDocument {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
}

// CanvasNodeData is what actually lives in a React Flow Node's `data` field — the
// UI-side working state. Distinct from CanvasNode (the serialized shape) on purpose:
// this is where a future ticket could add UI-only concerns (e.g. a "just added,
// highlight me" flag) without that ever leaking into CanvasDocument, since
// serialize.ts only ever reads the three fields below out of it.
// SimState is UI-only presentation state from a PC-88 "kill this node" simulation
// result — never read by serialize.ts, never present in CanvasDocument. "killed" is
// the node the user actually targeted; "severed" mirrors /simulate's own
// severed_paths exactly (a stateful node whose path no longer survives); "cascaded"
// is any OTHER node in /simulate's cascade[] (e.g. an unreachable non-stateful node)
// — kept visually distinct from "severed" so the one true acceptance signal
// (severed_paths) is never diluted by a broader "affected" set.
export type SimState = "normal" | "killed" | "severed" | "cascaded";

// journeyOnPath/utilization/notAssessableLoad (PC-127) are, like simState above,
// UI-only presentation state derived at render time from /simulate's own
// flow_detail/load — never written into CanvasDocument, never computed by this app
// (utilization is the exact number core.ComponentLoad already returned; this file
// only picks a display band for it, it never derives the number itself).
export interface CanvasNodeData extends Record<string, unknown> {
  nodeType: NodeType;
  label: string;
  capability: Record<string, string>;
  sizing?: Record<string, string>;
  serviceID?: string;
  securityGroupRules?: CanvasSecurityGroupRule[];
  availabilityZone?: string;
  cidrBlock?: string;
  routes?: CanvasRoute[];
  naclRules?: CanvasNACLRule[];
  simState?: SimState;
  journeyOnPath?: boolean;
  utilization?: number | null;
  notAssessableLoad?: boolean;
}

export interface CanvasEdgeData extends Record<string, unknown> {
  edgeType: EdgeType;
  severed?: boolean;
  journeyOnPath?: boolean;
}
