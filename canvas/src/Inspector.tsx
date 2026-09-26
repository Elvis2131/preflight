import { useEffect, useState } from "react";
import type { Node } from "@xyflow/react";
import type { CanvasNodeData } from "./types";
import { SIZING_FIELDS, type SizingFieldDef } from "./sizingFields";
import { listPricingSnapshots, getPricingSnapshot, listServiceCatalog, type PricingSnapshot, type ServiceCatalogEntry } from "./api";

// SERVICE_FOR_INSTANCE_TYPE_KIND maps a SizingFieldDef's own instanceTypeKind to the
// real AWS Price List `service` name PC-117's own cost engine matches against
// (core/cost.go: AmazonRDS, AmazonElastiCache; AmazonEC2 is not priced by that engine
// yet — PC-117's own costOneNode only switches on managed_database/cache/
// load_balancer — but the instance-type field is still real IR/contract vocabulary
// (PC-115), and filtering it by whatever a snapshot actually has is honest even
// before EC2 pricing exists: an empty filtered list for AmazonEC2 today is a real,
// correct "this snapshot prices no EC2 instance types," not a bug in this picker).
const SERVICE_FOR_INSTANCE_TYPE_KIND: Record<NonNullable<SizingFieldDef["instanceTypeKind"]>, string> = {
  compute: "AmazonEC2",
  managed_database: "AmazonRDS",
  cache: "AmazonElastiCache",
};

// usePricingSnapshot fetches the active pricing snapshot once (PC-116's own
// GET /pricing/snapshots + GET /pricing/snapshots/{id}) — null while loading or when
// none exists/is active, never thrown to the caller: "no snapshot" is this ticket's
// own explicitly-permitted fallback case ("If no snapshot exists yet, allow free
// text and flag it as unpriced"), not an error state.
function usePricingSnapshot(): { snapshot: PricingSnapshot | null; loading: boolean } {
  const [snapshot, setSnapshot] = useState<PricingSnapshot | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await listPricingSnapshots();
        const active = list.find((s) => s.active);
        if (!active) {
          if (!cancelled) setLoading(false);
          return;
        }
        const full = await getPricingSnapshot(active.id);
        if (!cancelled) setSnapshot(full);
      } catch {
        // Network/backend unavailable, or no pricing endpoint at all — fall back to
        // free text silently; this picker is a convenience, not a hard dependency.
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return { snapshot, loading };
}

// useServiceCatalog fetches the real registry's own node mappings once (PC-136's
// GET /catalog/services) — empty on any failure, never thrown to the caller: this
// picker is a convenience over free text, the same "no hard dependency" discipline
// usePricingSnapshot above already established for the instance-type picker.
function useServiceCatalog(): { entries: ServiceCatalogEntry[]; loading: boolean } {
  const [entries, setEntries] = useState<ServiceCatalogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await listServiceCatalog();
        if (!cancelled) setEntries(list);
      } catch {
        // Network/backend unavailable — fall back to free text silently.
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return { entries, loading };
}

function instanceTypesFor(snapshot: PricingSnapshot | null, kind: SizingFieldDef["instanceTypeKind"]): string[] {
  if (!snapshot || !kind) return [];
  const service = SERVICE_FOR_INSTANCE_TYPE_KIND[kind];
  const types = new Set<string>();
  for (const e of snapshot.entries) {
    if (e.service !== service) continue;
    const t = e.sku_attributes["instanceType"];
    if (t) types.add(t);
  }
  return Array.from(types).sort();
}

const labelStyle: React.CSSProperties = { display: "block", fontSize: 11, fontWeight: 600, marginTop: 10, marginBottom: 2 };
const inputStyle: React.CSSProperties = { width: "100%", fontSize: 12, padding: "4px 6px", boxSizing: "border-box" };

// Inspector is PC-110's own new panel: sizing fields for the currently-selected
// node, per NodeType (SIZING_FIELDS). Absent from this codebase before this ticket —
// there was no per-node inspector of any kind (capability entry has no UI either;
// out of this ticket's own scope, which is sizing specifically). Values are stated
// input with Kind=stated, source "canvas" — the same boundary PC-86 already
// established for every other canvas-entered value; this component only ever writes
// plain strings into CanvasNodeData.sizing, never a default for a field left blank.
export function Inspector({
  node,
  onChange,
  onServiceChange,
}: {
  node: Node<CanvasNodeData>;
  onChange: (nodeID: string, sizing: Record<string, string>) => void;
  onServiceChange: (nodeID: string, serviceID: string) => void;
}) {
  const fields = SIZING_FIELDS[node.data.nodeType];
  const { snapshot, loading } = usePricingSnapshot();
  const { entries: serviceEntries, loading: servicesLoading } = useServiceCatalog();
  const sizing = node.data.sizing ?? {};

  // servicesForNodeType (PC-136): only entries whose OWN node_type matches the
  // selected node's declared NodeType are offered — the same discipline
  // resolveCanvasCapabilityLevel enforces server-side (a mismatched ServiceID is
  // never trusted), applied here so the picker cannot even present a choice that
  // would fail server-side resolution.
  const servicesForNodeType = serviceEntries.filter((e) => e.node_type === node.data.nodeType);

  const setField = (key: string, value: string) => {
    const next = { ...sizing };
    if (value === "") {
      delete next[key];
    } else {
      next[key] = value;
    }
    onChange(node.id, next);
  };

  return (
    <aside style={{ width: 260, borderLeft: "1px solid #e2e8f0", padding: 12, overflowY: "auto" }}>
      <h2 style={{ fontSize: 14, margin: "0 0 4px" }}>Inspector</h2>
      <p style={{ fontSize: 11, color: "#64748b", margin: "0 0 8px" }}>
        {node.data.label} <span style={{ color: "#94a3b8" }}>({node.data.nodeType})</span>
      </p>

      <label style={labelStyle}>
        Service
        {!servicesLoading && servicesForNodeType.length === 0 && (
          <span style={{ fontWeight: 400, color: "#b45309" }}> (no real service mapped to this node type yet)</span>
        )}
      </label>
      <p style={{ fontSize: 10, color: "#94a3b8", margin: "0 0 4px" }}>
        Which real AWS/Azure service this node represents (PC-136). A journey through
        a node with no service picked stays not_assessable — this engine only
        resolves a capability level for a real, chosen service, never a guess from
        the structural type alone.
      </p>
      <select
        style={inputStyle}
        value={node.data.serviceID ?? ""}
        onChange={(e) => onServiceChange(node.id, e.target.value)}
      >
        <option value="">— none selected —</option>
        {servicesForNodeType.map((s) => (
          <option key={s.resource_type} value={s.resource_type}>
            {s.resource_type}
          </option>
        ))}
      </select>

      {!fields || fields.length === 0 ? (
        <p style={{ fontSize: 11, color: "#94a3b8" }}>
          No cost-relevant sizing is modelled for this node type.
        </p>
      ) : (
        <>
          <p style={{ fontSize: 11, color: "#64748b" }}>
            Sizing (PC-115/PC-117). Blank means unknown — the cost result reports{" "}
            <code>cost_unknown</code> for that dimension, never a guessed default.
          </p>
          {fields.map((f) => {
            const value = sizing[f.key] ?? "";
            const types = f.instanceTypeKind ? instanceTypesFor(snapshot, f.instanceTypeKind) : [];
            const showPicker = f.instanceTypeKind && types.length > 0;
            return (
              <div key={f.key}>
                <label style={labelStyle}>
                  {f.label}
                  {f.instanceTypeKind && !loading && !showPicker && (
                    <span style={{ fontWeight: 400, color: "#b45309" }}> (unpriced — no active pricing snapshot)</span>
                  )}
                </label>
                {showPicker ? (
                  <select style={inputStyle} value={value} onChange={(e) => setField(f.key, e.target.value)}>
                    <option value="">— select —</option>
                    {types.map((t) => (
                      <option key={t} value={t}>
                        {t}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    style={inputStyle}
                    type={f.kind === "int" ? "number" : "text"}
                    value={value}
                    onChange={(e) => setField(f.key, e.target.value)}
                    placeholder={f.kind === "int" ? "e.g. 100" : "e.g. free text"}
                  />
                )}
              </div>
            );
          })}
        </>
      )}
    </aside>
  );
}
