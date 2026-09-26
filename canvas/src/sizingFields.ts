import type { NodeType } from "./goldenVocabulary";

// SIZING_FIELDS is PC-110's own per-NodeType sizing field list for the Design
// inspector — mirrored from core.Sizing's own fields (preflight/core/ir.go) and
// which NodeType each applies to (that struct's own field-level doc comments), not
// invented here. Keys are core.Sizing's own canonical field names — the exact keys
// ingest/canvas.go's buildCanvasSizing reads, so the inspector never needs its own
// translation layer. A NodeType absent from this map (network_boundary, dns,
// object_store, queue/stream, identity, external_dependency) has no cost-relevant
// sizing dimension modelled at all — the inspector shows no sizing section for it,
// rather than a form with nothing meaningful to enter.
export interface SizingFieldDef {
  key: string;
  label: string;
  kind: "text" | "int";
  // instanceTypeKind, when set, marks this field as filterable by a pricing
  // snapshot's own SKU rows (PC-116) for this AWS service — see PricingContext's own
  // instanceTypesFor.
  instanceTypeKind?: "compute" | "managed_database" | "cache";
}

export const SIZING_FIELDS: Partial<Record<NodeType, SizingFieldDef[]>> = {
  compute: [
    { key: "instance_type", label: "Instance type", kind: "text", instanceTypeKind: "compute" },
    { key: "count", label: "Count", kind: "int" },
  ],
  container_workload: [
    { key: "task_cpu", label: "Task vCPU", kind: "text" },
    { key: "task_memory", label: "Task memory", kind: "text" },
    { key: "count", label: "Count", kind: "int" },
  ],
  managed_database: [
    { key: "instance_class", label: "Instance class", kind: "text", instanceTypeKind: "managed_database" },
    { key: "allocated_storage_gb", label: "Allocated storage (GB)", kind: "int" },
    { key: "storage_type", label: "Storage type", kind: "text" },
  ],
  cache: [
    { key: "cache_node_type", label: "Cache node type", kind: "text", instanceTypeKind: "cache" },
    { key: "count", label: "Count", kind: "int" },
  ],
  load_balancer: [{ key: "load_balancer_type", label: "Load balancer type", kind: "text" }],
};
