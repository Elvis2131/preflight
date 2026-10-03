import { useEffect, useState } from "react";
import type { Node } from "@xyflow/react";
import type { CanvasNodeData, CanvasSecurityGroupRule, CanvasRoute, CanvasNACLRule } from "./types";
import { SIZING_FIELDS, type SizingFieldDef } from "./sizingFields";
import { resolveRegion, rowInRegion, REGION_PRICED_TYPES } from "./region";
import { labelForService } from "./awsIcons";
import { listPricingSnapshots, getPricingSnapshot, listServiceCatalog, type PricingSnapshot, type ServiceCatalogEntry } from "./api";
import { ServiceMark } from "./ServiceMark";
import { awsOnlyCatalog } from "./awsCatalog";
import { attachedResources, configurationSpecs, type ConfigurationSpec, type ServiceNode, type ServiceEdge } from "./serviceConfiguration";

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

// useServiceCatalog fetches the backend mappings once and merges them with the local
// AWS catalog, so the design UI remains useful while the backend is being restarted.
function useServiceCatalog(): { entries: ServiceCatalogEntry[]; loading: boolean } {
  const [entries, setEntries] = useState<ServiceCatalogEntry[]>(awsOnlyCatalog([]));
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const list = await listServiceCatalog();
        if (!cancelled) setEntries(awsOnlyCatalog(list));
      } catch {
        if (!cancelled) setEntries(awsOnlyCatalog([]));
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

// instanceTypesFor lists the types the active snapshot can price — FILTERED BY THE RESOLVED
// REGION (PC-110). With no resolved region it returns nothing: the architect must choose
// one first, and the first of several declared regions is never assumed.
function instanceTypesFor(snapshot: PricingSnapshot | null, kind: SizingFieldDef["instanceTypeKind"], region: string | null): string[] {
  if (!snapshot || !kind || region === null) return [];
  const service = SERVICE_FOR_INSTANCE_TYPE_KIND[kind];
  const types = new Set<string>();
  for (const e of snapshot.entries) {
    if (e.service !== service || !rowInRegion(e.region, region)) continue;
    const t = e.sku_attributes["instanceType"];
    if (t) types.add(t);
  }
  return Array.from(types).sort();
}

const labelStyle: React.CSSProperties = { display: "block", fontSize: 11, fontWeight: 600, marginTop: 10, marginBottom: 2 };
const inputStyle: React.CSSProperties = { width: "100%", fontSize: 12, padding: "4px 6px", boxSizing: "border-box" };

const rowStyle: React.CSSProperties = { display: "flex", gap: 4, alignItems: "center", marginBottom: 4 };

// Rules are stated input on a real security-group resource, whether edited from
// its canvas node or inline in an attached service's settings. The server evaluates
// them; this editor never computes a security verdict.
function SecurityGroupRulesEditor({
  rules,
  onChange,
  groups = [],
}: {
  rules: CanvasSecurityGroupRule[];
  onChange: (rules: CanvasSecurityGroupRule[]) => void;
  groups?: Array<{ id: string; label: string }>;
}) {
  const updateRule = (i: number, patch: Partial<CanvasSecurityGroupRule>) => {
    const next = rules.slice();
    next[i] = { ...next[i], ...patch };
    onChange(next);
  };
  const removeRule = (i: number) => onChange(rules.filter((_, idx) => idx !== i));
  const addRule = () => onChange([...rules, { direction: "ingress", protocol: "tcp" }]);

  return (
    <div style={{ marginTop: 10, paddingTop: 8, borderTop: "1px solid var(--line)" }} data-testid="security-group-rules-editor">
      <label style={labelStyle}>Security group rules</label>
      <p style={{ fontSize: 10, color: "var(--subtle)", margin: "0 0 4px" }}>
        Set which traffic can enter or leave the attached services. Without rules,
        the simulation cannot assess this security group.
      </p>
      {rules.map((r, i) => (
        <div key={i} style={{ marginBottom: 6, paddingBottom: 6, borderBottom: "1px dashed var(--line-strong)" }}>
          <div style={rowStyle}>
            <select style={inputStyle} aria-label="Traffic direction" value={r.direction} onChange={(e) => updateRule(i, { direction: e.target.value as CanvasSecurityGroupRule["direction"] })}>
              <option value="ingress">Inbound traffic</option>
              <option value="egress">Outbound traffic</option>
            </select>
            <button aria-label={`Remove rule ${i + 1}`} onClick={() => removeRule(i)}>×</button>
          </div>
          <div style={rowStyle}>
            <input
              style={{ ...inputStyle, width: 60 }}
              placeholder="protocol"
              aria-label="Protocol"
              value={r.protocol}
              onChange={(e) => updateRule(i, { protocol: e.target.value })}
            />
            <input
              style={{ ...inputStyle, width: 55 }}
              type="number"
              placeholder="from"
              aria-label="From port"
              value={r.from_port ?? ""}
              onChange={(e) => updateRule(i, { from_port: e.target.value === "" ? undefined : Number(e.target.value) })}
            />
            <input
              style={{ ...inputStyle, width: 55 }}
              type="number"
              placeholder="to"
              aria-label="To port"
              value={r.to_port ?? ""}
              onChange={(e) => updateRule(i, { to_port: e.target.value === "" ? undefined : Number(e.target.value) })}
            />
          </div>
          <input
            style={{ ...inputStyle, marginTop: 4 }}
            aria-label="Allowed IP ranges"
            placeholder="cidr_blocks (comma-separated)"
            value={(r.cidr_blocks ?? []).join(", ")}
            onChange={(e) => {
              const cidrs = e.target.value.split(",").map((c) => c.trim()).filter((c) => c !== "");
              updateRule(i, { cidr_blocks: cidrs.length > 0 ? cidrs : undefined, source_security_group: cidrs.length > 0 ? undefined : r.source_security_group });
            }}
          />
          <select
            style={{ ...inputStyle, marginTop: 4 }}
            aria-label="Source security group"
            value={r.source_security_group ?? ""}
            onChange={(e) => updateRule(i, { source_security_group: e.target.value || undefined, cidr_blocks: e.target.value ? undefined : r.cidr_blocks })}
          >
            <option value="">Or choose a source security group…</option>
            {groups.map((group) => <option key={group.id} value={group.id}>{group.label}</option>)}
            {r.source_security_group && !groups.some((group) => group.id === r.source_security_group) && <option value={r.source_security_group}>Unresolved group ({r.source_security_group})</option>}
          </select>
        </div>
      ))}
      <button onClick={addRule}>+ rule</button>
    </div>
  );
}

// RoutesEditor (PC-138) authors the routes on a route-table node: destination CIDR to a
// target that is an internet gateway or NAT gateway node. Every field is stated input
// written verbatim into CanvasNodeData.routes — no routing logic here (route selection,
// longest-prefix match, public/private are all the server's). Targets offered are only
// the kinds the engine models; the server rejects anything else regardless.
function RoutesEditor({
  routes,
  targets,
  onChange,
}: {
  routes: CanvasRoute[];
  targets: Array<{ id: string; label: string }>;
  onChange: (routes: CanvasRoute[]) => void;
}) {
  const update = (i: number, patch: Partial<CanvasRoute>) => {
    const next = routes.slice();
    next[i] = { ...next[i], ...patch };
    onChange(next);
  };
  return (
    <div style={{ marginTop: 10, paddingTop: 8, borderTop: "1px solid var(--line)" }} data-testid="routes-editor">
      <label style={labelStyle}>Routes</label>
      <p style={{ fontSize: 10, color: "var(--subtle)", margin: "0 0 4px" }}>
        Choose where traffic from the attached subnets goes. Add a destination range
        and an internet or NAT gateway. Without a route table, routing stays unknown.
      </p>
      {targets.length === 0 && (
        <p style={{ fontSize: 10, color: "var(--warning-ink)", margin: "0 0 4px" }}>
          No internet gateway or NAT gateway in this design to route to — add one (service aws_internet_gateway / aws_nat_gateway).
        </p>
      )}
      {routes.map((r, i) => (
        <div key={i} style={{ ...rowStyle, marginBottom: 6 }}>
          <input
            style={{ ...inputStyle, width: 110 }}
            placeholder="destination CIDR"
            value={r.destination_cidr}
            onChange={(e) => update(i, { destination_cidr: e.target.value })}
          />
          <select style={inputStyle} value={r.target} onChange={(e) => update(i, { target: e.target.value })}>
            <option value="">— target —</option>
            {targets.map((t) => (
              <option key={t.id} value={t.id}>
                {t.label}
              </option>
            ))}
          </select>
          <button onClick={() => onChange(routes.filter((_, j) => j !== i))}>×</button>
        </div>
      ))}
      <button onClick={() => onChange([...routes, { destination_cidr: "", target: targets[0]?.id ?? "" }])}>+ route</button>
    </div>
  );
}

// NACLRulesEditor (PC-139) authors the ordered rules on a network-ACL node. Rules are
// evaluated lowest number first by the server (AWS: "Rules are evaluated starting with
// the lowest numbered rule. As soon as a rule matches traffic, it's applied"); nothing
// is evaluated here. No rule at all means not authored — never an implied allow or deny.
// DefaultNACLNotice (PC-153) says what "default network ACL" means. Display-side explanation only:
// which subnets use it, and what the engine does, are decided server-side. Wording follows the VPC
// User Guide, "Default network ACL for a VPC".
function DefaultNACLNotice({ authored }: { authored: boolean }) {
  return (
    <div data-testid="default-nacl-notice" style={{ fontSize: 11, color: "var(--ink-secondary)", background: "var(--surface-soft)", borderRadius: 6, padding: 8, margin: "0 0 8px" }}>
      <strong>Default network ACL.</strong> A VPC comes with one. Every subnet in this VPC that you have <em>not</em> associated
      with another network ACL uses it. Assign its VPC in the settings above so the engine knows
      which VPC it belongs to; a VPC has one default.
      {authored ? (
        <> The rules below <strong>replace</strong> AWS's default rules for those subnets.</>
      ) : (
        <> No rules authored: the engine assumes AWS's default (rule 100 allows all traffic in and out), and says so wherever it relies on it.</>
      )}
    </div>
  );
}

// The authorable NACL rule numbers (EC2 CreateNetworkAclEntry: 1-32766; 32767-65535 is
// reserved, and the catch-all deny lives there, engine-owned). Display-side guidance only —
// the server's NETCTL-NACL-RANGE check decides.
const NACL_RULE_NUMBER_MIN = 1;
const NACL_RULE_NUMBER_MAX = 32766;
const numberInRange = (n: number) => Number.isInteger(n) && n >= NACL_RULE_NUMBER_MIN && n <= NACL_RULE_NUMBER_MAX;

function NACLRulesEditor({ rules, onChange }: { rules: CanvasNACLRule[]; onChange: (rules: CanvasNACLRule[]) => void }) {
  const update = (i: number, patch: Partial<CanvasNACLRule>) => {
    const next = rules.slice();
    next[i] = { ...next[i], ...patch };
    onChange(next);
  };
  const nextNumber = rules.reduce((m, r) => Math.max(m, r.number), 0) + 10;
  return (
    <div style={{ marginTop: 10, paddingTop: 8, borderTop: "1px solid var(--line)" }} data-testid="nacl-editor">
      <label style={labelStyle}>Network ACL rules</label>
      <p style={{ fontSize: 10, color: "var(--subtle)", margin: "0 0 4px" }}>
        Control traffic at the subnet boundary. Rules run from the lowest number;
        the first match applies. Return traffic needs its own rule. Empty rules stay unknown.
      </p>
      {rules.map((r, i) => (
        <div key={i} style={{ marginBottom: 6, paddingBottom: 6, borderBottom: "1px dashed var(--line-strong)" }}>
          <div style={rowStyle}>
            <input
              style={{ ...inputStyle, width: 62, ...(numberInRange(r.number) ? {} : { borderColor: "var(--danger-ink)" }) }}
              type="number"
              min={NACL_RULE_NUMBER_MIN}
              max={NACL_RULE_NUMBER_MAX}
              placeholder="#"
              title={`Rule numbers ${NACL_RULE_NUMBER_MIN}-${NACL_RULE_NUMBER_MAX}`}
              value={r.number}
              onChange={(e) => update(i, { number: Number(e.target.value) })}
            />
            <select style={inputStyle} value={r.direction} onChange={(e) => update(i, { direction: e.target.value as CanvasNACLRule["direction"] })}>
              <option value="ingress">ingress</option>
              <option value="egress">egress</option>
            </select>
            <select style={inputStyle} value={r.action} onChange={(e) => update(i, { action: e.target.value as CanvasNACLRule["action"] })}>
              <option value="allow">allow</option>
              <option value="deny">deny</option>
            </select>
            <button onClick={() => onChange(rules.filter((_, j) => j !== i))}>×</button>
          </div>
          <div style={rowStyle}>
            <input style={{ ...inputStyle, width: 60 }} placeholder="protocol" value={r.protocol} onChange={(e) => update(i, { protocol: e.target.value })} />
            <input
              style={{ ...inputStyle, width: 55 }}
              type="number"
              placeholder="from"
              value={r.from_port ?? ""}
              onChange={(e) => update(i, { from_port: e.target.value === "" ? undefined : Number(e.target.value) })}
            />
            <input
              style={{ ...inputStyle, width: 55 }}
              type="number"
              placeholder="to"
              value={r.to_port ?? ""}
              onChange={(e) => update(i, { to_port: e.target.value === "" ? undefined : Number(e.target.value) })}
            />
          </div>
          <input style={{ ...inputStyle, marginTop: 4 }} placeholder="cidr_block (source for ingress, destination for egress)" value={r.cidr_block} onChange={(e) => update(i, { cidr_block: e.target.value })} />
        </div>
      ))}
      {rules.some((r) => !numberInRange(r.number)) && (
        <p style={{ fontSize: 10, color: "var(--danger-ink)", margin: "0 0 4px" }}>
          Rule numbers must be {NACL_RULE_NUMBER_MIN}-{NACL_RULE_NUMBER_MAX} (AWS reserves 32767-65535). The server rejects anything outside it.
        </p>
      )}
      <div style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 6px" }} data-testid="nacl-catchall">
        <strong>*</strong> &nbsp;deny all &nbsp;— the engine's own final rule. Always present; it cannot be added, edited or deleted here.
      </div>
      <button onClick={() => onChange([...rules, { direction: "ingress", number: Math.min(nextNumber, NACL_RULE_NUMBER_MAX), protocol: "tcp", cidr_block: "", action: "allow" }])}>+ rule</button>
    </div>
  );
}

interface ConfigurationEditorProps {
  nodes: ServiceNode[];
  edges: ServiceEdge[];
  onRename: (nodeID: string, name: string) => void;
  onAttach: (nodeID: string, spec: ConfigurationSpec, resourceIDs: string[]) => void;
  onCreateResource: (nodeID: string, spec: ConfigurationSpec, name: string) => string;
  onSecurityGroupRulesChange: (nodeID: string, rules: CanvasSecurityGroupRule[]) => void;
  onPlacementChange: (nodeID: string, patch: { availabilityZone?: string; cidrBlock?: string }) => void;
  routeTargets: Array<{ id: string; label: string }>;
  onRoutesChange: (nodeID: string, routes: CanvasRoute[]) => void;
  onNACLRulesChange: (nodeID: string, rules: CanvasNACLRule[]) => void;
}

const CONFIGURATION_GUIDES: Record<string, string> = {
  security_groups: "Rules that control traffic to and from this service.",
  iam_role: "The identity this service uses to access other AWS services.",
  subnets: "Choose the network locations where this service runs.",
  vpc: "The private network that contains this subnet.",
  db_subnet_group: "A collection of subnets available to this database.",
  cache_subnet_group: "A collection of subnets available to this cache.",
  route_table: "Destinations and gateways for traffic leaving this subnet.",
  network_acl: "Traffic rules applied at the subnet boundary.",
  internet_gateway: "The gateway connecting this VPC to the internet.",
  nat_gateway: "A gateway for outbound connections from private subnets.",
};

function ResourceSettings({ node, ...props }: ConfigurationEditorProps & { node: ServiceNode }) {
  return (
    <>
      <label className="setting-label">
        Resource name
        <input value={node.data.label} onChange={(e) => props.onRename(node.id, e.target.value)} />
      </label>
      {node.data.serviceID === "aws_security_group" && (
        <SecurityGroupRulesEditor rules={node.data.securityGroupRules ?? []} groups={props.nodes.filter((n) => n.data.serviceID === "aws_security_group").map((n) => ({ id: n.id, label: n.data.label }))} onChange={(rules) => props.onSecurityGroupRulesChange(node.id, rules)} />
      )}
      {(node.data.serviceID === "aws_vpc" || node.data.serviceID === "aws_subnet") && (
        <>
          <label className="setting-label">CIDR block
            <input placeholder="e.g. 10.0.1.0/24" value={node.data.cidrBlock ?? ""} onChange={(e) => props.onPlacementChange(node.id, { cidrBlock: e.target.value })} />
          </label>
          {node.data.serviceID === "aws_subnet" && (
            <label className="setting-label">Availability Zone
              <input placeholder="e.g. eu-west-1a" value={node.data.availabilityZone ?? ""} onChange={(e) => props.onPlacementChange(node.id, { availabilityZone: e.target.value })} />
            </label>
          )}
          {node.data.subnetFact && <p className="setting-help">{node.data.subnetFact.visibility === "not_assessable" ? "Network visibility is unknown" : `Routing: ${node.data.subnetFact.visibility}`}.</p>}
        </>
      )}
      {node.data.serviceID === "aws_route_table" && <RoutesEditor routes={node.data.routes ?? []} targets={props.routeTargets} onChange={(routes) => props.onRoutesChange(node.id, routes)} />}
      {node.data.serviceID === "aws_network_acl" && <NACLRulesEditor rules={node.data.naclRules ?? []} onChange={(rules) => props.onNACLRulesChange(node.id, rules)} />}
    </>
  );
}

// These controls edit the same resource nodes and edges as the detailed canvas.
// Expanding a shared resource edits it in place for every attached service.
function ServiceConfiguration({ node, ancestry = [], ...props }: ConfigurationEditorProps & { node: ServiceNode; ancestry?: string[] }) {
  const [expanded, setExpanded] = useState<string | null>(null);
  const [creating, setCreating] = useState<string | null>(null);
  const [name, setName] = useState("");
  const specs = configurationSpecs(node);
  return (
    <div className="service-configuration">
      {specs.map((spec) => {
        const assigned = attachedResources(node.id, spec, props.nodes, props.edges);
        const assignedIDs = assigned.map((resource) => resource.id);
        const available = props.nodes.filter((resource) => resource.data.serviceID === spec.serviceID && !assignedIDs.includes(resource.id) && !ancestry.includes(resource.id) && resource.id !== node.id);
        return (
          <section className="configuration-section" key={spec.id} data-testid={`configuration-${spec.id}`}>
            <h3>{spec.label}</h3>
            <p className="setting-help">{CONFIGURATION_GUIDES[spec.id]}</p>
            {assigned.length === 0 && <div className="setting-empty">None assigned</div>}
            {assigned.map((resource) => {
              const isOpen = expanded === resource.id;
              const sharedBy = new Set(props.edges.filter((edge) => edge.data?.edgeType === spec.edgeType && (spec.reverse ? edge.source : edge.target) === resource.id).map((edge) => spec.reverse ? edge.target : edge.source)).size;
              return (
                <div className="attached-resource" key={resource.id}>
                  <div className="attached-resource-row">
                    <ServiceMark serviceID={resource.data.serviceID!} size={22} />
                    <button className="resource-name" onClick={() => setExpanded(isOpen ? null : resource.id)} aria-expanded={isOpen} aria-label={`Edit ${resource.data.label}`}>
                      <span>{resource.data.label || labelForService(resource.data.serviceID!)}</span><span aria-hidden>{isOpen ? "⌃" : "⌄"}</span>
                    </button>
                    <button className="remove-assignment" title="Remove assignment; keep the resource" aria-label={`Unassign ${resource.data.label}`} onClick={() => props.onAttach(node.id, spec, assignedIDs.filter((id) => id !== resource.id))}>×</button>
                  </div>
                  {isOpen && (
                    <div className="attached-resource-editor" data-testid="attached-resource-editor">
                      {sharedBy > 1 && <p className="setting-help shared-resource-note">Shared by {sharedBy} services. Changes apply to each.</p>}
                      <ResourceSettings node={resource} {...props} />
                      {!ancestry.includes(resource.id) && <ServiceConfiguration node={resource} ancestry={[...ancestry, node.id]} {...props} />}
                    </div>
                  )}
                </div>
              );
            })}
            <div className="configuration-actions">
              <select value="" aria-label={`Assign ${spec.label}`} onChange={(e) => {
                if (e.target.value) props.onAttach(node.id, spec, spec.multiple ? [...assignedIDs, e.target.value] : [e.target.value]);
              }}>
                <option value="">{assigned.length && !spec.multiple ? "Replace with existing…" : "Choose existing…"}</option>
                {available.map((resource) => <option key={resource.id} value={resource.id}>{resource.data.label}</option>)}
              </select>
              <button aria-label={`Create ${spec.label}`} aria-expanded={creating === spec.id} onClick={() => { setCreating(creating === spec.id ? null : spec.id); setName(""); }}>+ New</button>
            </div>
            {creating === spec.id && (
              <form className="create-resource-form" onSubmit={(e) => {
                e.preventDefault();
                if (!name.trim()) return;
                setExpanded(props.onCreateResource(node.id, spec, name.trim()));
                setCreating(null);
                setName("");
              }}>
                <label className="setting-label">Name<input autoFocus required placeholder={`Name this ${labelForService(spec.serviceID).toLowerCase()}`} value={name} onChange={(e) => setName(e.target.value)} /></label>
                <div className="configuration-actions"><button type="submit" disabled={!name.trim()}>Create & assign</button><button type="button" onClick={() => setCreating(null)}>Cancel</button></div>
              </form>
            )}
          </section>
        );
      })}
    </div>
  );
}

// Inspector is PC-110's own new panel: sizing fields for the currently-selected
// node, per NodeType (SIZING_FIELDS). Absent from this codebase before this ticket —
// there was no per-node inspector of any kind (capability entry has no UI either;
// out of this ticket's own scope, which is sizing specifically). Values are stated
// input with Kind=stated, source "canvas" — the same boundary PC-86 already
// established for every other canvas-entered value; this component only ever writes
// plain strings into CanvasNodeData.sizing, never a default for a field left blank.
export function Inspector({
  node,
  onClose,
  onChange,
  onServiceChange,
  onSecurityGroupRulesChange,
  onPlacementChange,
  routeTargets,
  onRoutesChange,
  onNACLRulesChange,
  workloadRegions,
  nodes,
  edges,
  onRename,
  onAttach,
  onCreateResource,
}: ConfigurationEditorProps & {
  node: Node<CanvasNodeData>;
  onClose: () => void;
  onChange: (nodeID: string, sizing: Record<string, string>) => void;
  onServiceChange: (nodeID: string, serviceID: string) => void;
  onSecurityGroupRulesChange: (nodeID: string, rules: CanvasSecurityGroupRule[]) => void;
  onPlacementChange: (nodeID: string, patch: { availabilityZone?: string; cidrBlock?: string }) => void;
  routeTargets: Array<{ id: string; label: string }>;
  onRoutesChange: (nodeID: string, routes: CanvasRoute[]) => void;
  onNACLRulesChange: (nodeID: string, rules: CanvasNACLRule[]) => void;
  workloadRegions: string[];
}) {
  const fields = SIZING_FIELDS[node.data.nodeType];
  const { snapshot, loading } = usePricingSnapshot();
  const { entries: serviceEntries, loading: servicesLoading } = useServiceCatalog();
  const sizing = node.data.sizing ?? {};

  // Region (PC-110): resolved per component — its own region, else the workload's only when
  // it declares exactly one, else unresolved (the architect must choose; the component is
  // unpriced until they do). Display-side guidance: the server prices and decides.
  const pricedByRegion = REGION_PRICED_TYPES.has(node.data.nodeType);
  const regionResolution = resolveRegion(sizing["region"], workloadRegions);
  const resolvedRegion = regionResolution.resolved ? regionResolution.region : null;

  // servicesForNodeType (PC-136): only entries whose OWN node_type matches the
  // selected node's declared NodeType are offered. The UI is AWS-only for now.
  // resolveCanvasCapabilityLevel enforces server-side (a mismatched ServiceID is
  // never trusted), applied here so the picker cannot even present a choice that
  // would fail server-side resolution.
  const servicesForNodeType = serviceEntries.filter((e) => e.node_type === node.data.nodeType && e.resource_type.startsWith("aws_"));

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
    <aside className="workspace-panel inspector-panel" style={{ width: 310, borderLeft: "1px solid var(--line)", padding: 16, overflowY: "auto" }}>
      <div className="inspector-heading"><h2 style={{ fontSize: 14, margin: "0 0 4px" }}>Service settings</h2><button aria-label="Close service settings" onClick={onClose}>×</button></div>
      <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
        {node.data.label} <span style={{ color: "var(--subtle)" }}>({node.data.nodeType})</span>
      </p>

      <label className="setting-label">Name<input value={node.data.label} onChange={(e) => onRename(node.id, e.target.value)} /></label>

      <label style={labelStyle}>
        Service
        {!servicesLoading && servicesForNodeType.length === 0 && (
          <span style={{ fontWeight: 400, color: "var(--warning-ink)" }}> (no real service mapped to this node type yet)</span>
        )}
      </label>
      <p style={{ fontSize: 10, color: "var(--subtle)", margin: "0 0 4px" }}>
        Pick the AWS service this box represents. Without a service, the backend
        cannot know the real capability level, so checks that depend on it stay
        not_assessable.
      </p>
      {node.data.serviceID && (
        <div style={{ display: "flex", alignItems: "center", gap: 6, margin: "0 0 4px", fontSize: 12 }}>
          <ServiceMark serviceID={node.data.serviceID} size={24} />
          <span>{labelForService(node.data.serviceID!)}</span>
        </div>
      )}
      <select
        style={inputStyle}
        value={node.data.serviceID ?? ""}
        onChange={(e) => onServiceChange(node.id, e.target.value)}
      >
        <option value="">— none selected —</option>
        {servicesForNodeType.map((s) => (
          <option key={s.resource_type} value={s.resource_type}>
            {labelForService(s.resource_type)}
          </option>
        ))}
      </select>

      {node.data.serviceID && serviceEntries.find((s) => s.resource_type === node.data.serviceID)?.capability_level === "UNMODELED" && (
        <div className="guide-card compact" role="note">
          Available for architecture design. Results use the core component model;
          checks that require this service’s specific behaviour remain unknown.
        </div>
      )}

      {Object.keys(node.data.capability).some((key) => key.startsWith("design_")) && (
        <details className="architecture-notes" data-testid="architecture-notes">
          <summary>Architecture notes</summary>
          <p className="setting-help">Template intent and deployment choices. These notes do not establish a simulation result.</p>
          {Object.entries(node.data.capability).filter(([key]) => key.startsWith("design_")).map(([key, value]) => {
            let formatted = value;
            try { formatted = JSON.stringify(JSON.parse(value), null, 2); } catch { /* Plain text notes remain plain text. */ }
            return <div key={key}><strong>{key.slice(7).replaceAll("_", " ")}</strong><pre>{formatted}</pre></div>;
          })}
        </details>
      )}

      {configurationSpecs(node).length > 0 && (
        <>
          <p className="configuration-intro">Manage access and network settings here. Click an assigned resource to edit its details.</p>
          <ServiceConfiguration key={node.id} node={node} nodes={nodes} edges={edges} onRename={onRename} onAttach={onAttach} onCreateResource={onCreateResource}
            onSecurityGroupRulesChange={onSecurityGroupRulesChange} onPlacementChange={onPlacementChange} routeTargets={routeTargets} onRoutesChange={onRoutesChange} onNACLRulesChange={onNACLRulesChange} />
        </>
      )}

      {(node.data.serviceID === "aws_vpc" || node.data.serviceID === "aws_subnet") && (
        <div data-testid="placement-editor">
          <label style={labelStyle}>CIDR block</label>
          <input
            style={inputStyle}
            placeholder="e.g. 10.0.1.0/24"
            value={node.data.cidrBlock ?? ""}
            onChange={(e) => onPlacementChange(node.id, { cidrBlock: e.target.value })}
          />
          {node.data.serviceID === "aws_subnet" && (
            <>
              <label style={labelStyle}>Availability Zone</label>
              <input
                style={inputStyle}
                placeholder="e.g. eu-west-1a"
                value={node.data.availabilityZone ?? ""}
                onChange={(e) => onPlacementChange(node.id, { availabilityZone: e.target.value })}
              />
            </>
          )}
          <p style={{ fontSize: 11, color: "var(--muted)" }}>
            Choose network placement in the settings above. Blank values stay unknown.
          </p>
        </div>
      )}

      {node.data.serviceID === "aws_route_table" && (
        <RoutesEditor routes={node.data.routes ?? []} targets={routeTargets} onChange={(r) => onRoutesChange(node.id, r)} />
      )}

      {node.data.serviceID === "aws_default_network_acl" && <DefaultNACLNotice authored={(node.data.naclRules ?? []).length > 0} />}
      {(node.data.serviceID === "aws_network_acl" || node.data.serviceID === "aws_default_network_acl") && (
        <NACLRulesEditor rules={node.data.naclRules ?? []} onChange={(r) => onNACLRulesChange(node.id, r)} />
      )}

      {node.data.nodeType === "network_boundary" && (!node.data.serviceID || node.data.serviceID === "aws_security_group") && (
        <SecurityGroupRulesEditor
          rules={node.data.securityGroupRules ?? []}
          groups={nodes.filter((n) => n.data.serviceID === "aws_security_group").map((n) => ({ id: n.id, label: n.data.label }))}
          onChange={(rules) => onSecurityGroupRulesChange(node.id, rules)}
        />
      )}

      {!fields || fields.length === 0 ? (
        <p style={{ fontSize: 11, color: "var(--subtle)" }}>
          No cost-relevant sizing is modelled for this node type.
        </p>
      ) : (
        <>
          <p style={{ fontSize: 11, color: "var(--muted)" }}>
            Size this service for cost estimates. Leave a field blank if you do not
            know its value; the estimate will show that cost as unknown.
          </p>
          {pricedByRegion && (
            <div data-testid="region-block">
              <label style={labelStyle}>Region</label>
              <input
                style={inputStyle}
                list="workload-regions"
                placeholder={workloadRegions.length === 1 ? `blank = the workload's ${workloadRegions[0]}` : "e.g. eu-west-1"}
                value={sizing["region"] ?? ""}
                onChange={(e) => setField("region", e.target.value.trim())}
              />
              <datalist id="workload-regions">
                {workloadRegions.map((r) => (
                  <option key={r} value={r} />
                ))}
              </datalist>
              <p style={{ fontSize: 10, margin: "2px 0 0", color: regionResolution.resolved ? "var(--muted)" : "var(--warning-ink)" }} data-testid="region-resolution">
                {regionResolution.resolved
                  ? `Priced in ${regionResolution.region} (${regionResolution.source === "component" ? "this component's own region" : "the workload's only declared region"}).`
                  : `${regionResolution.reason}. Until you choose one this component is unpriced (cost_unknown) — never a guessed default.`}
              </p>
            </div>
          )}
          {fields.map((f) => {
            const value = sizing[f.key] ?? "";
            const types = f.instanceTypeKind ? instanceTypesFor(snapshot, f.instanceTypeKind, resolvedRegion) : [];
            const showPicker = f.instanceTypeKind && types.length > 0;
            return (
              <div key={f.key}>
                <label style={labelStyle}>
                  {f.label}
                  {f.instanceTypeKind && !loading && !showPicker && (
                    <span style={{ fontWeight: 400, color: "var(--warning-ink)" }}>
                      {resolvedRegion === null && pricedByRegion
                        ? " (unpriced — choose a region first)"
                        : snapshot
                          ? ` (unpriced — the active snapshot has no price for this in ${resolvedRegion ?? "any region"})`
                          : " (unpriced — no active pricing snapshot)"}
                    </span>
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
