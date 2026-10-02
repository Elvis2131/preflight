import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import {
  NODE_TYPES,
  NODE_TYPE_LABELS,
  EDGE_TYPES,
  EDGE_TYPE_LABELS,
  EDGE_TYPE_GUIDES,
  DEFAULT_EDGE_TYPE,
  type NodeType,
  type EdgeType,
} from "./goldenVocabulary";
import type { CanvasNodeData, CanvasEdgeData, CanvasSecurityGroupRule, CanvasRoute, CanvasNACLRule } from "./types";
import { serialize } from "./serialize";
import { GoldenNode } from "./GoldenNode";
import { Inspector } from "./Inspector";
import { CONTAINER_SIZE, containerRank, reevaluate } from "./containment";
import { templateToCanvasState } from "./templateLoader";
import { MODES, capabilities, type Mode } from "./modes";
import { AnalyzeView } from "./analyze/AnalyzeView";
import { listTemplates, getTemplate, simulateFaults, evaluateScenarios, deriveCanvas, listServiceCatalog, type ServiceCatalogEntry, type TemplateMeta, type Fault, type ScenarioResult, type SubnetFact } from "./api";
import { FailureLab } from "./FailureLab";
import { GroupingNode } from "./GroupingNode";
import { labelForService } from "./awsIcons";
import { ServiceMark } from "./ServiceMark";
import { awsOnlyCatalog, type AWSServiceCatalogEntry } from "./awsCatalog";
import { buildGroupings } from "./groupings";
import { killedTargets } from "./faultBuilder";
import { assessCanvas, simulateNodeLoss, simulateBaseline, describeSimError, type SimulateResponse } from "./api";
import { ReportView } from "./ReportView";
import { WorkloadForm, buildWorkload, emptyWorkloadFormValue, workloadToFormValue, type WorkloadFormValue } from "./WorkloadForm";
import { JourneyPanel } from "./JourneyPanel";

const nodeTypes = { golden: GoldenNode, grouping: GroupingNode };

let nextID = 1;
function freshID(prefix: string): string {
  return `${prefix}-${nextID++}`;
}

// Core types organise the library; only services can be placed. The directory
// supplies design choices while the backend registry supplies actual model coverage.
function Palette() {
  const [services, setServices] = useState<ServiceCatalogEntry[]>([]);
  const [query, setQuery] = useState("");
  useEffect(() => {
    let cancelled = false;
    listServiceCatalog()
      .then((l) => !cancelled && setServices(awsOnlyCatalog(l)))
      .catch(() => !cancelled && setServices(awsOnlyCatalog([])));
    return () => {
      cancelled = true;
    };
  }, []);

  const onDragStart = (event: React.DragEvent, nodeType: NodeType, serviceID: string) => {
    event.dataTransfer.setData("application/preflight-node-type", nodeType);
    event.dataTransfer.setData("application/preflight-service-id", serviceID);
    event.dataTransfer.effectAllowed = "move";
  };

  const itemStyle: React.CSSProperties = {
    padding: "9px 10px",
    marginBottom: 7,
    borderRadius: 9,
    border: "1px solid #e4e8ee",
    background: "#ffffff",
    cursor: "grab",
    fontSize: 12,
  };

  // Sorted by display label, so the list never depends on the server's iteration order.
  const sortedServices = awsOnlyCatalog(services).sort((a, b) => labelForService(a.resource_type).localeCompare(labelForService(b.resource_type)));
  const normalizedQuery = query.trim().toLowerCase();
  const componentGroups = NODE_TYPES.map((nt) => {
    const coreMatches = NODE_TYPE_LABELS[nt].toLowerCase().includes(normalizedQuery);
    const groupServices = sortedServices.filter((svc) => svc.node_type === nt);
    const services = groupServices.filter(
      (svc) =>
        !normalizedQuery ||
        coreMatches ||
        labelForService(svc.resource_type).toLowerCase().includes(normalizedQuery) ||
        svc.display_name?.toLowerCase().includes(normalizedQuery) ||
        svc.category?.toLowerCase().includes(normalizedQuery) ||
        svc.resource_type.toLowerCase().includes(normalizedQuery),
    );
    return { nodeType: nt, coreMatches, services };
  }).filter((g) => !normalizedQuery || g.coreMatches || g.services.length > 0);

  return (
    <aside className="workspace-panel service-library" aria-label="AWS service library">
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
        <h2 style={{ fontSize: 14, margin: "0 0 4px" }}>AWS services</h2>
        <span style={{ color: "#8b95a7", fontSize: 10 }}>{sortedServices.length} available</span>
      </div>
      <p style={{ fontSize: 11, color: "#697386", margin: "0 0 12px", lineHeight: 1.45 }}>
        Browse a group, then drag an AWS service onto the canvas.
      </p>
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search AWS services…"
        aria-label="Search AWS services"
        style={{ width: "100%", boxSizing: "border-box", padding: "8px 10px", marginBottom: 16, border: "1px solid #e4e8ee", borderRadius: 8, background: "#f8fafc" }}
      />
      <div className="palette-section-label">Core components</div>
      <div data-testid="service-palette">
        {componentGroups.map((group) => (
          <ComponentServiceGroup
            key={group.nodeType}
            nodeType={group.nodeType}
            services={group.services}
            searching={!!normalizedQuery}
            onDragStart={onDragStart}
            itemStyle={itemStyle}
          />
        ))}
      </div>
      {componentGroups.length === 0 && <div style={{ color: "#8b95a7", fontSize: 11, padding: "4px 0 12px" }}>No matching components or AWS services.</div>}
    </aside>
  );
}

function ComponentServiceGroup({
  nodeType,
  services,
  searching,
  onDragStart,
  itemStyle,
}: {
  nodeType: NodeType;
  services: AWSServiceCatalogEntry[];
  searching: boolean;
  onDragStart: (e: React.DragEvent, nodeType: NodeType, serviceID: string) => void;
  itemStyle: React.CSSProperties;
}) {
  const serviceCards = (items: AWSServiceCatalogEntry[]) => items.map((svc) => (
    <div
      key={svc.resource_type}
      draggable
      onDragStart={(e) => onDragStart(e, svc.node_type as NodeType, svc.resource_type)}
      style={{ ...itemStyle, display: "flex", alignItems: "center", gap: 8 }}
      data-service={svc.resource_type}
      title={`${labelForService(svc.resource_type)} · Drag to place${svc.capability_level === "UNMODELED" ? " · Service-specific checks not available yet" : ""}`}
    >
      <ServiceMark serviceID={svc.resource_type} size={22} />
      <span>{labelForService(svc.resource_type)}</span>
    </div>
  ));
  const supportingGroups = Array.from(new Set(services.map((svc) => svc.category ?? "Other services"))).sort();
  return (
    <details key={`${nodeType}-${searching}`} open={searching} className="palette-component-group" data-group={nodeType}>
      <summary
        draggable={false}
        className="palette-group-heading"
        data-component={nodeType}
      >
        <span>{NODE_TYPE_LABELS[nodeType]}</span>
        <span className="palette-count">{services.length}</span>
      </summary>
      {services.length > 0 && (
        <div className="palette-service-list">
          {nodeType === "external_dependency" ? supportingGroups.map((category) => (
            <details key={`${category}-${searching}`} open={searching} className="palette-supporting-group">
              <summary>{category}</summary>
              {serviceCards(services.filter((svc) => (svc.category ?? "Other services") === category))}
            </details>
          )) : serviceCards(services)}
        </div>
      )}
    </details>
  );
}

function CanvasInner() {
  const [nodes, setNodes, onNodesChange] = useNodesState<Node<CanvasNodeData>>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge<CanvasEdgeData>>([]);
  const [pendingEdgeType, setPendingEdgeType] = useState<EdgeType>(DEFAULT_EDGE_TYPE);
  const [showJSON, setShowJSON] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { screenToFlowPosition } = useReactFlow();

  // PC-88: click a node, kill it, see what /simulate says breaks. sessionID is
  // generated once per page load — one canvas session per browser tab, good enough
  // for this ticket's own scope (no persistence, no multi-tab story exists yet).
  const sessionIDRef = useRef(crypto.randomUUID());
  const [workloadForm, setWorkloadForm] = useState<WorkloadFormValue>(emptyWorkloadFormValue());
  const [showWorkloadForm, setShowWorkloadForm] = useState(true);
  // mode (PC-104): one workspace, five modes (Design / Simulate / Failure Lab / Analyze /
  // Report). Authoring is Design's alone (modes.ts encodes who may do what); the others
  // only read what the server computed. Every mode shares THIS component's state — the
  // canvas, the workload form, the session id and latestVersion below — so switching
  // modes can never lose the design or which session/version is being looked at.
  const [mode, setMode] = useState<Mode>("design");
  const caps = capabilities(mode);
  // latestVersion is the newest version this session has stored (set whenever a mode
  // assesses the current design); null until something has been assessed.
  const [latestVersion, setLatestVersion] = useState<number | null>(null);
  const [selectedNodeID, setSelectedNodeID] = useState<string | null>(null);
  const [simBusy, setSimBusy] = useState(false);
  const [simError, setSimError] = useState<string | null>(null);
  const [simSummary, setSimSummary] = useState<SimulateResponse | null>(null);
  // selectedJourneyID (PC-127): which declared journey the JourneyPanel/canvas
  // highlight currently reflects — independent of selectedNodeID (killing a node and
  // inspecting a journey's flow are separate concerns).
  const [selectedJourneyID, setSelectedJourneyID] = useState<string | null>(null);
  const [showJourneyPanel, setShowJourneyPanel] = useState(true);

  // pickingJourneyRowIndex (PC-124's own explicit acceptance criterion: "journey
  // paths can be picked on the canvas by clicking components in order") — while set,
  // a node click appends that node's ID to the named journey row's own pathText
  // instead of the normal select-a-node-to-kill behavior below.
  const [pickingJourneyRowIndex, setPickingJourneyRowIndex] = useState<number | null>(null);
  const startPickPath = useCallback((rowIndex: number) => setPickingJourneyRowIndex(rowIndex), []);
  const stopPickPath = useCallback(() => setPickingJourneyRowIndex(null), []);

  const onNodeClick = useCallback<NodeMouseHandler>(
    (_event, node) => {
      if (pickingJourneyRowIndex !== null) {
        setWorkloadForm((v) => {
          const rows = v.journeyRows.slice();
          const row = rows[pickingJourneyRowIndex];
          if (!row) return v;
          const hops = row.pathText.split(",").map((h) => h.trim()).filter((h) => h !== "");
          hops.push(node.id);
          rows[pickingJourneyRowIndex] = { ...row, pathText: hops.join(", ") };
          return { ...v, journeyRows: rows };
        });
        return;
      }
      setSelectedNodeID(node.id);
    },
    [pickingJourneyRowIndex],
  );

  // STEP_DELAY_MS (PC-89) is a fixed UI pacing interval between reveal steps — purely
  // a legibility choice for how fast the animation advances on screen. It is NOT a
  // modeled propagation time: /simulate computes no duration, rate, or velocity for
  // failure propagation anywhere (Layer 2/3 remain P2 per PRD §5.5's own demotion,
  // undisturbed by this ticket — reviewed explicitly against that reasoning, not just
  // for visual polish, per this ticket's own third acceptance criterion). The UI only
  // ever displays a step COUNT ("step 2 of 4" — see animStep below), never a time,
  // duration, or rate, so nothing here implies a measured or simulated value the
  // engine never computed.
  const STEP_DELAY_MS = 500;
  const animationTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [animStep, setAnimStep] = useState<{ index: number; total: number } | null>(null);

  const stopAnimation = useCallback(() => {
    if (animationTimerRef.current !== null) {
      clearTimeout(animationTimerRef.current);
      animationTimerRef.current = null;
    }
    setAnimStep(null);
  }, []);

  useEffect(() => stopAnimation, [stopAnimation]);

  // Templates (PC-108): a reference architecture loads into the SAME node/edge/workload
  // state a hand-drawn design lives in. Nothing here assesses anything — the architect
  // runs the ordinary assessment afterwards, through the ordinary canvas pipeline.
  const [templateList, setTemplateList] = useState<TemplateMeta[]>([]);
  const [templateError, setTemplateError] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    listTemplates()
      .then((l) => !cancelled && setTemplateList(l))
      .catch(() => undefined); // no backend / no templates endpoint: the picker simply stays empty
    return () => {
      cancelled = true;
    };
  }, []);

  const clearSimulation = useCallback(() => {
    stopAnimation();
    setNodes((nds) =>
      nds.map((n) => ({ ...n, data: { ...n.data, simState: undefined } })),
    );
    setEdges((eds) =>
      eds.map((e) => ({ ...e, data: { ...e.data!, severed: undefined } })),
    );
    setSimSummary(null);
    setSimError(null);
  }, [setNodes, setEdges, stopAnimation]);

  const loadTemplate = useCallback(
    async (id: string) => {
      if (!id) return;
      if (nodes.length > 0 && !window.confirm("Replace the current canvas with this template?")) return;
      try {
        const t = await getTemplate(id);
        const { nodes: tn, edges: te } = templateToCanvasState(t);
        stopAnimation();
        setSelectedNodeID(null);
        setSimSummary(null);
        setSimError(null);
        setNodes(tn);
        setEdges(te);
        setWorkloadForm(workloadToFormValue(t.workload));
        setTemplateError(null);
      } catch (err) {
        setTemplateError(err instanceof Error ? err.message : String(err));
      }
    },
    [nodes.length, setNodes, setEdges, stopAnimation],
  );

  // applySimResult paints the /simulate response onto the current graph — presentation
  // only (PC-88's own Conversation: "the underlying verdict, severed_paths, and cascade
  // data are exactly what /simulate already returns, not reinterpreted or summarised
  // differently for the canvas"). PC-89 adds the animation itself: cascade[] is now
  // real hop-distance order from the point of failure (core.Simulate/CascadeOrder) —
  // this reveals exactly one cascade[] member per step, in cascade[]'s own returned
  // order, nothing invented or interpolated between steps (this ticket's own first
  // acceptance criterion, verbatim). severed_paths and cascade remain visually
  // DISTINCT states, not merged into one "affected" look, so the exact set PC-88's own
  // acceptance criterion checks (severed_paths) stays a legible, separate signal.
  const applySimResult = useCallback(
    (sim: SimulateResponse, killedIDs: string | string[]) => {
      const killed = new Set(Array.isArray(killedIDs) ? killedIDs : [killedIDs]);
      stopAnimation();
      const severed = new Set(sim.severed_paths ?? []);
      const order = sim.cascade ?? [];
      setSimSummary(sim);

      // Clean slate before animating — a second kill must not inherit stale
      // simState/severed flags a prior run left behind.
      setNodes((nds) => nds.map((n) => ({ ...n, data: { ...n.data, simState: undefined } })));
      setEdges((eds) => eds.map((e) => ({ ...e, data: { ...e.data!, severed: undefined } })));

      if (order.length === 0) return; // nothing to animate (e.g. an unaffected verdict)

      const revealed = new Set<string>();
      const revealStep = (i: number) => {
        if (i >= order.length) {
          setAnimStep(null);
          return;
        }
        revealed.add(order[i]);
        setAnimStep({ index: i + 1, total: order.length });
        setNodes((nds) =>
          nds.map((n) => {
            if (!revealed.has(n.id)) return n;
            let simState: CanvasNodeData["simState"] = "cascaded";
            if (killed.has(n.id)) simState = "killed";
            else if (severed.has(n.id)) simState = "severed";
            return { ...n, data: { ...n.data, simState } };
          }),
        );
        setEdges((eds) =>
          eds.map((e) => ({
            ...e,
            data: { ...e.data!, severed: revealed.has(e.source) || revealed.has(e.target) },
          })),
        );
        animationTimerRef.current = setTimeout(() => revealStep(i + 1), STEP_DELAY_MS);
      };
      revealStep(0);
    },
    [setNodes, setEdges, stopAnimation],
  );

  const killSelectedNode = useCallback(async () => {
    if (!selectedNodeID) return;
    setSimBusy(true);
    setSimError(null);
    try {
      // Re-assess the CURRENT canvas state fresh on every kill, deliberately — this
      // always simulates against what's on screen right now rather than risking a
      // stale version_number from an earlier edit (no client-side fault logic either
      // way: assessCanvas/simulateNodeLoss are both thin wrappers, see api.ts).
      const doc = serialize(nodes, edges);
      const workload = buildWorkload(workloadForm);
      const assessed = await assessCanvas(sessionIDRef.current, doc, workload);
      setLatestVersion(assessed.version_number);
      const sim = await simulateNodeLoss(
        sessionIDRef.current,
        assessed.version_number,
        selectedNodeID,
      );
      applySimResult(sim, selectedNodeID);
    } catch (err) {
      setSimError(describeSimError(err));
    } finally {
      setSimBusy(false);
    }
  }, [selectedNodeID, nodes, edges, workloadForm, applySimResult]);

  // runBaseline (PC-127) is the "before" half of the fault before/after comparison
  // the Card asks for: a real /simulate call with an empty faults list (a genuine
  // no-fault scenario core.Simulate already supports, not a client-side stand-in),
  // so the JourneyPanel/canvas highlight can show real flow/utilization even before
  // any node has been killed.
  const runBaseline = useCallback(async () => {
    setSimBusy(true);
    setSimError(null);
    try {
      const doc = serialize(nodes, edges);
      const workload = buildWorkload(workloadForm);
      const assessed = await assessCanvas(sessionIDRef.current, doc, workload);
      setLatestVersion(assessed.version_number);
      const sim = await simulateBaseline(sessionIDRef.current, assessed.version_number);
      stopAnimation();
      setSimSummary(sim);
    } catch (err) {
      setSimError(describeSimError(err));
    } finally {
      setSimBusy(false);
    }
  }, [nodes, edges, workloadForm, stopAnimation]);

  // Failure Lab scenarios (PC-131). Running assesses the CURRENT design fresh (a new
  // version), then asks the server to simulate the declared faults against it — the
  // canvas only ever names faults, never decides what they break.
  const [scenarioResults, setScenarioResults] = useState<ScenarioResult[] | null>(null);
  const [scenarioResultsVersion, setScenarioResultsVersion] = useState<number | null>(null);

  const runFaults = useCallback(
    async (faults: Fault[]) => {
      setSimBusy(true);
      setSimError(null);
      try {
        const doc = serialize(nodes, edges);
        const workload = buildWorkload(workloadForm);
        const assessed = await assessCanvas(sessionIDRef.current, doc, workload);
        setLatestVersion(assessed.version_number);
        const sim = await simulateFaults(sessionIDRef.current, assessed.version_number, faults);
        applySimResult(sim, killedTargets(faults));
      } catch (err) {
        setSimError(describeSimError(err));
      } finally {
        setSimBusy(false);
      }
    },
    [nodes, edges, workloadForm, applySimResult],
  );

  // rerunAllScenarios re-evaluates every SAVED scenario against the design as it is now:
  // assess the current canvas (a new version), then have the server run each saved
  // definition against that version. Nothing here replays an earlier result.
  const rerunAllScenarios = useCallback(async () => {
    setSimBusy(true);
    setSimError(null);
    try {
      const doc = serialize(nodes, edges);
      const workload = buildWorkload(workloadForm);
      const assessed = await assessCanvas(sessionIDRef.current, doc, workload);
      setLatestVersion(assessed.version_number);
      setScenarioResults(await evaluateScenarios(sessionIDRef.current, assessed.version_number));
      setScenarioResultsVersion(assessed.version_number);
    } catch (err) {
      setSimError(describeSimError(err));
    } finally {
      setSimBusy(false);
    }
  }, [nodes, edges, workloadForm]);

  // repriceCurrentDesign (PC-123) re-runs assessCanvas against the CURRENT canvas
  // state, no explicit price_snapshot_id — the server's own already-documented
  // resolution rule (server/assess.go) then prices against whatever snapshot is
  // ACTIVE right now. This creates a brand new version; it never mutates any
  // previously-stored report (server/report.go/GetReport only ever reads a version
  // back, it has no update path at all).
  const repriceCurrentDesign = useCallback(async (): Promise<number> => {
    const doc = serialize(nodes, edges);
    const workload = buildWorkload(workloadForm);
    const assessed = await assessCanvas(sessionIDRef.current, doc, workload);
    setLatestVersion(assessed.version_number);
    return assessed.version_number;
  }, [nodes, edges, workloadForm]);

  const selectedNodeLabel = useMemo(
    () => nodes.find((n) => n.id === selectedNodeID)?.data.label,
    [nodes, selectedNodeID],
  );

  const selectedNode = useMemo(
    () => nodes.find((n) => n.id === selectedNodeID) ?? null,
    [nodes, selectedNodeID],
  );

  // updateNodeSizing (PC-110) is the Inspector's own write path — mirrors every
  // other node-data update in this file (setNodes with an immutable map), never a
  // default substituted for a field the architect left blank (Inspector itself
  // already deletes a key on blank input; this just stores whatever it hands back).
  const updateNodeSizing = useCallback(
    (nodeID: string, sizing: Record<string, string>) => {
      setNodes((nds) => nds.map((n) => (n.id === nodeID ? { ...n, data: { ...n.data, sizing } } : n)));
    },
    [setNodes],
  );

  // updateNodeServiceID (PC-136) is the Inspector's Service picker's own write
  // path — same immutable-map pattern as updateNodeSizing above. An empty selection
  // stores undefined (not ""), matching serialize.ts's own "omitted, not blank"
  // convention for service_id.
  const updateNodeServiceID = useCallback(
    (nodeID: string, serviceID: string) => {
      const next = nodes.map((n) => {
        if (n.id !== nodeID) return n;
        const rank = containerRank(serviceID);
        // A VPC/subnet becomes a drawn container (PC-105): sized, and layered behind
        // the nodes dropped inside it (VPC furthest back). Any other service — or none —
        // returns the node to its ordinary size and layer.
        const size = CONTAINER_SIZE[serviceID];
        const { style: _style, zIndex: _z, ...rest } = n;
        void _style;
        void _z;
        return {
          ...rest,
          ...(rank > 0 && size ? { style: { width: size.width, height: size.height }, zIndex: -(4 - rank) } : {}),
          data: {
            ...n.data,
            serviceID: serviceID || undefined,
            label: serviceID && n.data.label === (n.data.serviceID ? labelForService(n.data.serviceID) : NODE_TYPE_LABELS[n.data.nodeType])
              ? labelForService(serviceID) : n.data.label,
          },
        };
      });
      setNodes(next);
      // Becoming (or ceasing to be) a container changes what sits inside what.
      setEdges((eds) => reevaluate(next, eds, next.map((n) => n.id)));
    },
    [nodes, setNodes, setEdges],
  );

  // PC-138/PC-139: the Inspector's route and NACL editors' own write paths — same
  // immutable-map pattern as the other inspector write paths.
  const updateNodeRoutes = useCallback(
    (nodeID: string, routes: CanvasRoute[]) => {
      setNodes((nds) => nds.map((n) => (n.id === nodeID ? { ...n, data: { ...n.data, routes } } : n)));
    },
    [setNodes],
  );
  const updateNodeNACLRules = useCallback(
    (nodeID: string, naclRules: CanvasNACLRule[]) => {
      setNodes((nds) => nds.map((n) => (n.id === nodeID ? { ...n, data: { ...n.data, naclRules } } : n)));
    },
    [setNodes],
  );
  // Route targets the engine models: internet-gateway and NAT-gateway nodes on the canvas.
  const routeTargets = useMemo(
    () =>
      nodes
        .filter((n) => n.data.serviceID === "aws_internet_gateway" || n.data.serviceID === "aws_nat_gateway")
        .map((n) => ({ id: n.id, label: `${n.data.label} (${n.data.serviceID})` })),
    [nodes],
  );

  // updateNodePlacement (PC-105) is the Inspector's CIDR/AZ fields' write path — same
  // immutable-map pattern as the other inspector write paths.
  const updateNodePlacement = useCallback(
    (nodeID: string, patch: { availabilityZone?: string; cidrBlock?: string }) => {
      setNodes((nds) =>
        nds.map((n) =>
          n.id === nodeID
            ? {
                ...n,
                data: {
                  ...n.data,
                  ...("availabilityZone" in patch ? { availabilityZone: patch.availabilityZone || undefined } : {}),
                  ...("cidrBlock" in patch ? { cidrBlock: patch.cidrBlock || undefined } : {}),
                },
              }
            : n,
        ),
      );
    },
    [setNodes],
  );

  // onNodeDragStop (PC-105): where a node was dropped IS its containment. A dropped
  // resource gets its contained_in edge to the innermost VPC/subnet under it; dropping
  // a container re-evaluates everything, since moving it can take nodes in or out.
  // Geometry only — whether the resulting placement is valid is the server's decision.
  const onNodeDragStop = useCallback(
    (_: unknown, dragged: Node<CanvasNodeData>) => {
      setEdges((eds) => {
        const current = nodes.map((n) => (n.id === dragged.id ? { ...n, position: dragged.position } : n));
        const ids = containerRank(dragged.data.serviceID) > 0 ? current.map((n) => n.id) : [dragged.id];
        return reevaluate(current, eds, ids);
      });
    },
    [nodes, setEdges],
  );

  // updateNodeSecurityGroupRules (PC-137) is the Inspector's SecurityGroupRulesEditor
  // own write path — same immutable-map pattern as updateNodeSizing/updateNodeServiceID.
  const updateNodeSecurityGroupRules = useCallback(
    (nodeID: string, rules: CanvasSecurityGroupRule[]) => {
      setNodes((nds) =>
        nds.map((n) => (n.id === nodeID ? { ...n, data: { ...n.data, securityGroupRules: rules } } : n)),
      );
    },
    [setNodes],
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) =>
        addEdge(
          {
            ...connection,
            type: "default",
            label: pendingEdgeType,
            data: { edgeType: pendingEdgeType },
          },
          eds,
        ),
      );
    },
    [pendingEdgeType, setEdges],
  );

  const onDragOver = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
  }, []);

  const onDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const nodeType = event.dataTransfer.getData("application/preflight-node-type") as NodeType;
      if (!NODE_TYPES.includes(nodeType)) return;
      // Reject plain component types: only an AWS service can be placed.
      const serviceID = event.dataTransfer.getData("application/preflight-service-id");
      if (!serviceID.startsWith("aws_")) return;

      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const id = freshID(nodeType);
      const rank = serviceID ? containerRank(serviceID) : 0;
      const containerSize = serviceID ? CONTAINER_SIZE[serviceID] : undefined;
      const newNode: Node<CanvasNodeData> = {
        id,
        type: "golden",
        position,
        // A VPC / subnet dropped from the palette becomes a drawn container straight away,
        // exactly as choosing that service in the Inspector does.
        ...(rank > 0 && containerSize ? { style: { width: containerSize.width, height: containerSize.height }, zIndex: -(4 - rank) } : {}),
        data: {
          nodeType,
          label: serviceID ? labelForService(serviceID) : NODE_TYPE_LABELS[nodeType],
          capability: {},
          ...(serviceID ? { serviceID } : {}),
        },
      };
      setNodes((nds) => nds.concat(newNode));
      // A drop IS a placement (PC-105): a node dropped inside a VPC/subnet is contained_in it.
      setEdges((eds) => reevaluate(nodes.concat(newNode), eds, [id]));
    },
    [screenToFlowPosition, setNodes, setEdges, nodes],
  );

  const doc = serialize(nodes, edges);
  const journeys = useMemo(() => buildWorkload(workloadForm).journeys ?? [], [workloadForm]);

  // Default to the first declared journey once one exists and nothing is selected
  // yet — a convenience only; it never invents a selection when the architect has
  // declared no journeys at all.
  useEffect(() => {
    if (selectedJourneyID === null && journeys.length > 0) {
      setSelectedJourneyID(journeys[0].id);
    }
  }, [journeys, selectedJourneyID]);

  // selectedFlow/journeyNodeIDs/journeyHopKeys (PC-127) project the selected
  // journey's own Hops (from /simulate's flow_detail — never recomputed) onto the
  // current canvas: which node IDs and which from/to edge pairs to highlight. Purely
  // a lookup against server-returned hop pairs, not a flow computation of its own.
  const selectedFlow = simSummary?.flow_detail.find((f) => f.JourneyID === selectedJourneyID) ?? null;
  const journeyNodeIDs = useMemo(() => {
    const ids = new Set<string>();
    for (const hop of selectedFlow?.Hops ?? []) {
      ids.add(hop.From);
      ids.add(hop.To);
    }
    return ids;
  }, [selectedFlow]);
  const journeyHopKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const hop of selectedFlow?.Hops ?? []) {
      keys.add(`${hop.From}|${hop.To}`);
    }
    return keys;
  }, [selectedFlow]);
  const loadByNodeID = useMemo(() => {
    const loadEntries = simSummary?.load ?? [];
    const m = new Map<string, (typeof loadEntries)[number]>();
    for (const l of loadEntries) m.set(l.NodeID, l);
    return m;
  }, [simSummary]);

  // Derived, read-only facts about the design (PC-105): each subnet's AZ, region and route-
  // derived public/private classification, asked of the server (POST /canvas/derive — pure,
  // stores nothing, works on a half-drawn design) whenever the document changes. The picture
  // draws exactly what comes back; no mode derives any of it in the browser.
  const [subnetFacts, setSubnetFacts] = useState<SubnetFact[]>([]);
  const docKey = JSON.stringify(serialize(nodes, edges));
  useEffect(() => {
    if (nodes.length === 0) {
      setSubnetFacts([]);
      return;
    }
    let cancelled = false;
    const t = setTimeout(() => {
      deriveCanvas(JSON.parse(docKey))
        .then((r) => !cancelled && setSubnetFacts(r.subnets))
        .catch(() => !cancelled && setSubnetFacts([])); // no backend: simply no derived overlay
    }, 400);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
    // docKey is the whole serialized document: the derivation depends on nothing else.
  }, [docKey]); // eslint-disable-line react-hooks/exhaustive-deps
  const factByNode = useMemo(() => new Map(subnetFacts.map((f) => [f.node_id, f])), [subnetFacts]);

  // displayNodes overlays journey highlight + utilization presentation at render
  // time only, same "never written back into node state" discipline displayEdges
  // below already applies to severed.
  const displayNodes = [
    // Region/AZ groupings (PC-105): read-only boxes derived from the subnets' server-derived
    // facts. They exist only here — never in `nodes`, so never serialized or editable.
    ...buildGroupings(nodes, subnetFacts),
    ...nodes.map((n) => {
      const load = loadByNodeID.get(n.id);
      return {
        ...n,
        data: {
          ...n.data,
          journeyOnPath: journeyNodeIDs.has(n.id),
          utilization: load ? load.Utilization : undefined,
          notAssessableLoad: load ? load.Capacity === null : false,
          subnetFact: factByNode.get(n.id),
        },
      };
    }),
    // The grouping nodes are display-only (unselectable, undraggable, pointer-events off), so
    // no handler ever receives one; React Flow's own change application ignores an id that is
    // not in `nodes` state. The cast only reconciles that heterogeneous list with the canvas
    // handlers' node type.
  ] as unknown as Node<CanvasNodeData>[];

  // displayEdges applies severed styling at render time only — edges' own stored
  // `data.severed` (set by applySimResult) never becomes a React Flow `style`/
  // `animated` prop directly, so serialize.ts's own field whitelist stays the single
  // source of truth for what's UI-only vs. wire-shape.
  const displayEdges = edges.map((e) => {
    const onPath = journeyHopKeys.has(`${e.source}|${e.target}`);
    if (e.data?.severed) {
      return { ...e, style: { stroke: "#dc2626", strokeDasharray: "6 4" }, animated: true };
    }
    if (onPath) {
      return { ...e, style: { stroke: "#7c3aed", strokeWidth: 2.5 }, animated: true };
    }
    return e;
  });

  return (
    <div className="app-shell">
      <div
        style={{ display: "flex", gap: 6, padding: "6px 12px", borderBottom: "1px solid #cbd5e1", background: "#f8fafc", alignItems: "center" }}
        data-testid="mode-bar"
        className="mode-bar"
      >
        <strong style={{ marginRight: 12, fontSize: 14, letterSpacing: "-0.02em" }}>Preflight</strong>
        {MODES.map((m) => (
          <button
            key={m.id}
            data-mode={m.id}
            onClick={() => setMode(m.id)}
            className={`mode-button${mode === m.id ? " active" : ""}`}
          >
            {m.label}
          </button>
        ))}
        <span className="session-pill" style={{ marginLeft: "auto" }} data-testid="session-label">
          session {sessionIDRef.current.slice(0, 8)} · {latestVersion === null ? "no version assessed yet" : `latest v${latestVersion}`}
        </span>
      </div>
      {mode === "report" ? (
        <ReportView sessionID={sessionIDRef.current} onReprice={repriceCurrentDesign} />
      ) : mode === "analyze" ? (
        <AnalyzeView sessionID={sessionIDRef.current} latestVersion={latestVersion} />
      ) : (
    <>
      <div className="canvas-toolbar" role="toolbar" aria-label="Architecture tools">
        <div className="toolbar-main-row">
          {caps.canEditDesign && (
            <>
              <label className="toolbar-field template-field">
                <span>Start from a template</span>
                <select
                  value=""
                  aria-label="Architecture template"
                  disabled={templateList.length === 0}
                  onChange={(e) => void loadTemplate(e.target.value)}
                  data-testid="template-picker"
                >
                  <option value="">{templateList.length ? "Choose an architecture…" : "Templates unavailable"}</option>
                  {templateList.map((t) => <option key={t.id} value={t.id} title={t.description}>{t.name}</option>)}
                </select>
              </label>
              <label className="toolbar-field connection-field">
                <span>Connection</span>
                <select
                  value={pendingEdgeType}
                  aria-label="Connection type"
                  aria-describedby="connection-guide"
                  data-testid="connection-type"
                  onChange={(e) => setPendingEdgeType(e.target.value as EdgeType)}
                >
                  {EDGE_TYPES.map((et) => <option key={et} value={et}>{EDGE_TYPE_LABELS[et]}</option>)}
                </select>
              </label>
              <div className="toolbar-actions" aria-label="Canvas panels">
                <button
                  onClick={() => setShowWorkloadForm((v) => !v)}
                  aria-pressed={showWorkloadForm}
                  title="Set traffic, capacity and requirements for your design"
                >Workload</button>
                <button
                  onClick={() => setShowJSON((v) => !v)}
                  aria-pressed={showJSON}
                  data-testid="canvas-json-toggle"
                  title="View the architecture document"
                >View JSON</button>
              </div>
              <button className="primary-action toolbar-test-action" onClick={() => setMode("simulate")}>
                Test design <span aria-hidden>→</span>
              </button>
            </>
          )}
          {caps.canInjectFaults && (
            <button onClick={killSelectedNode} disabled={!selectedNodeID || simBusy} className="danger-action">
              {simBusy ? "Simulating..." : "Kill selected node"}
            </button>
          )}
          {caps.canRunBaseline && (
            <button onClick={runBaseline} disabled={simBusy || journeys.length === 0} className="primary-action" data-testid="run-baseline">
              Run baseline (no fault)
            </button>
          )}
          {(caps.canInjectFaults || caps.canRunBaseline) && (
            <button onClick={clearSimulation} disabled={!simSummary}>Clear simulation</button>
          )}
          {caps.showsJourneyPanel && (
            <button onClick={() => setShowJourneyPanel((v) => !v)} aria-pressed={showJourneyPanel}>
              Journey panel
            </button>
          )}
        </div>
        <div className="toolbar-context-row">
          {caps.canEditDesign ? (
            <span id="connection-guide">
              <span className="connection-guide-arrow" aria-hidden>↗</span>
              Drag between the dots on two services. {EDGE_TYPE_GUIDES[pendingEdgeType]}
            </span>
          ) : journeys.length === 0 && caps.canRunBaseline ? (
            <span>Add a journey in Design → Workload to run a baseline.</span>
          ) : (
            <span>Select a service on the canvas to inspect a failure.</span>
          )}
          <span className="canvas-count">{nodes.length} services · {edges.length} connections</span>
        </div>
        {templateError && <div className="toolbar-error" role="alert">{templateError}</div>}
        {(pickingJourneyRowIndex !== null || selectedNodeID) && (
          <div className="toolbar-selection" role="status">
            {pickingJourneyRowIndex !== null
              ? "Picking journey path — click services in order."
              : <>Selected: <strong>{selectedNodeLabel ?? selectedNodeID}</strong></>}
          </div>
        )}
      </div>
    <div className="workspace">
      {caps.showsAuthoringPanels && <Palette />}
      {caps.showsAuthoringPanels && showWorkloadForm && (
        <div className="workspace-panel" style={{ width: 300, borderRight: "1px solid #e2e8f0", overflowY: "auto" }}>
          <WorkloadForm
            value={workloadForm}
            onChange={setWorkloadForm}
            pickingPathForRow={pickingJourneyRowIndex}
            onStartPickPath={startPickPath}
            onStopPickPath={stopPickPath}
          />
        </div>
      )}
      <div className="canvas-column">
        {(simError || simSummary) && (
          <div
            className="status-strip"
            style={{
              fontSize: 12,
              background: simError ? "#fff5f5" : undefined,
            }}
          >
            {simError ? (
              <span style={{ color: "#dc2626" }}>/simulate error: {simError}</span>
            ) : (
              simSummary && (
                <span>
                  verdict: <strong>{String(simSummary.verdict.value ?? simSummary.verdict.reason)}</strong>{" "}
                  · severed_paths: [{(simSummary.severed_paths ?? []).join(", ") || "none"}] · cascade: [
                  {(simSummary.cascade ?? []).join(", ") || "none"}] · capacity:{" "}
                  {String(simSummary.capacity.value ?? simSummary.capacity.reason)}
                  {animStep && (
                    // A step COUNT, deliberately — never a duration/rate/time. See
                    // STEP_DELAY_MS's own doc comment for why.
                    <strong style={{ marginLeft: 10, color: "#0f172a" }}>
                      revealing step {animStep.index} of {animStep.total}
                    </strong>
                  )}
                </span>
              )
            )}
          </div>
        )}
        <div className="canvas-stage" ref={wrapperRef} onDragOver={caps.canEditDesign ? onDragOver : undefined} onDrop={caps.canEditDesign ? onDrop : undefined}>
          <ReactFlow
            nodes={displayNodes}
            edges={displayEdges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={onNodeClick}
            onNodeDragStop={onNodeDragStop}
            // A selected container must stay BEHIND the resources drawn inside it (PC-105):
            // React Flow's default raises a selected node, which would cover its own
            // children and make them unclickable.
            elevateNodesOnSelect={false}
            // Authoring is Design's alone (PC-104): in every other mode the canvas is a
            // read-only picture of what the server computed — selectable, never editable.
            nodesDraggable={caps.canEditDesign}
            nodesConnectable={caps.canEditDesign}
            deleteKeyCode={caps.canEditDesign ? "Backspace" : null}
            fitView
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>
          {showJSON && (
            <pre
              style={{
                position: "absolute",
                right: 8,
                top: 8,
                maxWidth: 420,
                maxHeight: "80%",
                overflow: "auto",
                background: "#0f172a",
                color: "#e2e8f0",
                padding: 12,
                borderRadius: 6,
                fontSize: 11,
                zIndex: 10,
              }}
            >
              {JSON.stringify(doc, null, 2)}
            </pre>
          )}
        </div>
      </div>
      {selectedNode && caps.showsAuthoringPanels && (
        <Inspector
          node={selectedNode}
          onChange={updateNodeSizing}
          onServiceChange={updateNodeServiceID}
          onSecurityGroupRulesChange={updateNodeSecurityGroupRules}
          onPlacementChange={updateNodePlacement}
          routeTargets={routeTargets}
          onRoutesChange={updateNodeRoutes}
          onNACLRulesChange={updateNodeNACLRules}
          workloadRegions={buildWorkload(workloadForm).regions ?? []}
        />
      )}
      {caps.canInjectFaults && (
        <div style={{ width: 340, borderLeft: "1px solid #e2e8f0", overflowY: "auto" }}>
          <FailureLab
            sessionID={sessionIDRef.current}
            nodes={nodes}
            edges={edges}
            regions={buildWorkload(workloadForm).regions ?? []}
            busy={simBusy}
            onRun={(f) => void runFaults(f)}
            onRerunAll={() => void rerunAllScenarios()}
            results={scenarioResults}
            resultsVersion={scenarioResultsVersion}
          />
        </div>
      )}
      {showJourneyPanel && caps.showsJourneyPanel && (
        <div style={{ width: 320, borderLeft: "1px solid #e2e8f0", overflowY: "auto" }}>
          <JourneyPanel
            journeys={journeys}
            flowDetail={simSummary?.flow_detail ?? []}
            latency={simSummary?.latency}
            load={simSummary?.load ?? []}
            selectedJourneyID={selectedJourneyID}
            onSelectJourney={setSelectedJourneyID}
          />
        </div>
      )}
    </div>
    </>
      )}
    </div>
  );
}

export default function App() {
  return (
    <ReactFlowProvider>
      <CanvasInner />
    </ReactFlowProvider>
  );
}
            nodeLabels={Object.fromEntries(nodes.map((n) => [n.id, n.data.label]))}
