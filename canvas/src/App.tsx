import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MarkerType,
  addEdge,
  useNodesState,
  useEdgesState,
  useReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeMouseHandler,
  type NodeChange,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import {
  NODE_TYPES,
  NODE_TYPE_LABELS,
  EDGE_TYPE_LABELS,
  type NodeType,
  type EdgeType,
} from "./goldenVocabulary";
import type { CanvasNodeData, CanvasEdgeData, CanvasSecurityGroupRule, CanvasRoute, CanvasNACLRule } from "./types";
import { serialize } from "./serialize";
import { TemplateConfirmation } from "./TemplateConfirmation";
import { ConnectionPicker } from "./ConnectionPicker";
import { ThemeToggle } from "./ThemeToggle";
import { GoldenNode } from "./GoldenNode";
import { Inspector } from "./Inspector";
import { CONTAINER_SIZE, containerRank } from "./containment";
import { applyCanvasNodeChanges } from "./containerResize";
import { templateToCanvasState } from "./templateLoader";
import { MODES, capabilities, type Mode } from "./modes";
import { AnalyzeView } from "./analyze/AnalyzeView";
import { listTemplates, getTemplate, simulateFaults, evaluateScenarios, deriveCanvas, listServiceCatalog, type ServiceCatalogEntry, type TemplateMeta, type Fault, type ScenarioResult, type SubnetFact } from "./api";
import { FailureLab } from "./FailureLab";
import { labelForService } from "./awsIcons";
import { ServiceMark } from "./ServiceMark";
import { awsOnlyCatalog, type AWSServiceCatalogEntry } from "./awsCatalog";
import { killedTargets } from "./faultBuilder";
import { assessCanvas, simulateNodeLoss, simulateBaseline, describeSimError, type SimulateResponse } from "./api";
import { ReportView } from "./ReportView";
import { WorkloadForm, buildWorkload, emptyWorkloadFormValue, workloadToFormValue, type WorkloadFormValue } from "./WorkloadForm";
import { JourneyPanel } from "./JourneyPanel";
import { TrafficEdge } from "./TrafficEdge";
import { TrafficPlayer } from "./TrafficPlayer";
import { SimulationStatus } from "./SimulationStatus";
import { useTrafficPlayback } from "./useTrafficPlayback";
import { dnsLookupForJourney, trafficHopKey } from "./trafficPlayback";
import { arrangeServiceView, attachedResources, configurationSummary, replaceAttachment, visibleServiceNodes, type ConfigurationSpec } from "./serviceConfiguration";

const nodeTypes = { golden: GoldenNode };
const edgeTypes = { traffic: TrafficEdge };

let nextID = 1;
function freshID(prefix: string): string {
  return `${prefix}-${nextID++}`;
}

// Core types organise the library; only services can be placed. The directory
// supplies design choices while the backend registry supplies actual model coverage.
function Palette({ children }: { children: React.ReactNode }) {
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
    border: "1px solid var(--line)",
    background: "var(--surface)",
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
      <div className="library-title"><span className="eyebrow">BUILD YOUR ARCHITECTURE</span><h2>Service library</h2><p>Find the building blocks for your system.</p></div>
      {children}
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
        <h3 className="library-provider"><span className="provider-dot" />AWS services</h3>
        <span style={{ color: "var(--subtle)", fontSize: 10 }}>{sortedServices.length} available</span>
      </div>
      <p className="library-drag-hint">Drag a service onto the canvas to add it.</p>
      <input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search AWS services…"
        aria-label="Search AWS services"
        style={{ width: "100%", boxSizing: "border-box", padding: "8px 10px", marginBottom: 16, border: "1px solid var(--line)", borderRadius: 8, background: "var(--surface-soft)" }}
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
      {componentGroups.length === 0 && <div style={{ color: "var(--subtle)", fontSize: 11, padding: "4px 0 12px" }}>No matching components or AWS services.</div>}
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
  const [nodes, setNodes] = useNodesState<Node<CanvasNodeData>>([]);
  const onNodesChange = useCallback((changes: NodeChange<Node<CanvasNodeData>>[]) => {
    setNodes((current) => applyCanvasNodeChanges(changes, current));
  }, [setNodes]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge<CanvasEdgeData>>([]);
  const [connectionPicker, setConnectionPicker] = useState<{
    connection: Connection; edgeID?: string; anchor: { x: number; y: number };
  } | null>(null);
  const [showJSON, setShowJSON] = useState(false);
  const [layoutVersion, setLayoutVersion] = useState(0);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { screenToFlowPosition, fitView } = useReactFlow();

  // PC-88: click a node, kill it, see what /simulate says breaks. sessionID is
  // generated once per page load — one canvas session per browser tab, good enough
  // for this ticket's own scope (no persistence, no multi-tab story exists yet).
  const sessionIDRef = useRef(crypto.randomUUID());
  const [workloadForm, setWorkloadForm] = useState<WorkloadFormValue>(emptyWorkloadFormValue());
  const [showWorkloadForm, setShowWorkloadForm] = useState(false);
  const [showLibrary, setShowLibrary] = useState(() => window.innerWidth > 640);
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
  const inspectorOpen = selectedNodeID !== null && caps.showsAuthoringPanels;
  const [simBusy, setSimBusy] = useState(false);
  const [simError, setSimError] = useState<string | null>(null);
  const [simSummary, setSimSummary] = useState<SimulateResponse | null>(null);
  const [simInputKey, setSimInputKey] = useState<string | null>(null);
  const [simRunKind, setSimRunKind] = useState<"baseline" | "fault">("baseline");
  const [trafficEnabled, setTrafficEnabled] = useState(false);
  // selectedJourneyID (PC-127): which declared journey the JourneyPanel/canvas
  // highlight currently reflects — independent of selectedNodeID (killing a node and
  // inspecting a journey's flow are separate concerns).
  const [selectedJourneyID, setSelectedJourneyID] = useState<string | null>(null);
  const [showJourneyPanel, setShowJourneyPanel] = useState(true);
  const simulationInputKey = JSON.stringify([serialize(nodes, edges), buildWorkload(workloadForm)]);
  const fitCanvas = useCallback(() => {
    const hops = trafficEnabled && caps.showsJourneyPanel && simInputKey === simulationInputKey
      ? simSummary?.flow_detail.find((f) => f.JourneyID === selectedJourneyID)?.Hops : undefined;
    const [canvas, workload] = JSON.parse(simulationInputKey);
    const dns = dnsLookupForJourney(canvas.nodes, workload.journeys?.find((j: { id: string }) => j.id === selectedJourneyID));
    const focusNodes = hops?.length ? [...new Set([...hops.flatMap((h) => [h.From, h.To]), ...(dns ? [dns.To] : [])])]
      .filter(Boolean).map((id) => ({ id: id === "internet" ? "traffic-display:internet" : id })) : undefined;
    return fitView({ nodes: focusNodes, padding: 0.18, maxZoom: 1,
      duration: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 250 });
  }, [fitView, trafficEnabled, caps.showsJourneyPanel, simSummary, simInputKey, simulationInputKey, selectedJourneyID]);

  // Fit after measuring a changed layout or opening a side panel. Selecting a
  // different service or editing a field does not disturb the current viewport.
  useEffect(() => {
    let secondFrame = 0;
    const firstFrame = requestAnimationFrame(() => {
      secondFrame = requestAnimationFrame(() => void fitCanvas());
    });
    return () => { cancelAnimationFrame(firstFrame); cancelAnimationFrame(secondFrame); };
  }, [layoutVersion, fitCanvas, inspectorOpen, showWorkloadForm, showJourneyPanel, mode]);

  useEffect(() => {
    const stage = wrapperRef.current;
    if (!stage) return;
    let frame = 0;
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => void fitCanvas());
    });
    observer.observe(stage);
    return () => { observer.disconnect(); cancelAnimationFrame(frame); };
  }, [fitCanvas, mode]);

  // pickingJourneyRowIndex (PC-124's own explicit acceptance criterion: "journey
  // paths can be picked on the canvas by clicking components in order") — while set,
  // a node click appends that node's ID to the named journey row's own pathText
  // instead of the normal select-a-node-to-kill behavior below.
  const [pickingJourneyRowIndex, setPickingJourneyRowIndex] = useState<number | null>(null);
  const startPickPath = useCallback((rowIndex: number) => setPickingJourneyRowIndex(rowIndex), []);
  const stopPickPath = useCallback(() => setPickingJourneyRowIndex(null), []);

  const onNodeClick = useCallback<NodeMouseHandler>(
    (_event, node) => {
      if (node.data.trafficExternal) return;
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
      if (caps.showsAuthoringPanels) setShowWorkloadForm(false);
    },
    [pickingJourneyRowIndex, caps.showsAuthoringPanels],
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
  const [templateListError, setTemplateListError] = useState<string | null>(null);
  const [templatesFetching, setTemplatesFetching] = useState(true);
  const [templateLoading, setTemplateLoading] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState<TemplateMeta | null>(null);
  const [templateRetry, setTemplateRetry] = useState(0);
  const [loadedTemplate, setLoadedTemplate] = useState<TemplateMeta | null>(null);
  useEffect(() => {
    let cancelled = false;
    listTemplates()
      .then((l) => { if (!cancelled) setTemplateList(l); })
      .catch((error) => { if (!cancelled) setTemplateListError(error instanceof Error ? error.message : "Could not load templates."); })
      .finally(() => { if (!cancelled) setTemplatesFetching(false); });
    return () => {
      cancelled = true;
    };
  }, [templateRetry]);

  const clearSimulation = useCallback(() => {
    stopAnimation();
    setNodes((nds) =>
      nds.map((n) => ({ ...n, data: { ...n.data, simState: undefined } })),
    );
    setEdges((eds) =>
      eds.map((e) => ({ ...e, data: { ...e.data!, severed: undefined } })),
    );
    setSimSummary(null);
    setSimInputKey(null);
    setTrafficEnabled(false);
    setSimError(null);
  }, [setNodes, setEdges, stopAnimation]);

  const loadTemplate = useCallback(
    async (id: string) => {
      if (!id) return;
      setTemplateLoading(true);
      setTemplateError(null);
      try {
        const t = await getTemplate(id);
        const { nodes: tn, edges: te } = templateToCanvasState(t);
        stopAnimation();
        setSelectedNodeID(null);
        setSimSummary(null);
        setSimInputKey(null);
        setTrafficEnabled(false);
        setSelectedJourneyID(null);
        setSimError(null);
        setNodes(arrangeServiceView(tn, te).map((n) => n.data.servicePosition ? { ...n, position: n.data.servicePosition } : n));
        setEdges(te);
        setLoadedTemplate(t.meta);
        setLayoutVersion((v) => v + 1);
        setWorkloadForm(workloadToFormValue(t.workload));
        setTemplateError(null);
      } catch (err) {
        setTemplateError(err instanceof Error ? err.message : String(err));
      } finally {
        setTemplateLoading(false);
      }
    },
    [setNodes, setEdges, stopAnimation],
  );

  const requestTemplate = (id: string) => {
    if (!id || templateLoading) return;
    const template = templateList.find((item) => item.id === id);
    if (!template) return;
    if (nodes.length || edges.length || loadedTemplate) setPendingTemplate(template);
    else void loadTemplate(id);
  };

  const clearArchitecture = () => {
    clearSimulation();
    setNodes([]);
    setEdges([]);
    setConnectionPicker(null);
    setSelectedNodeID(null);
    setSelectedJourneyID(null);
    setPickingJourneyRowIndex(null);
    setLoadedTemplate(null);
    setTemplateError(null);
    setWorkloadForm(emptyWorkloadFormValue());
    setLatestVersion(null);
    setScenarioResults(null);
    setScenarioResultsVersion(null);
  };

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
    (sim: SimulateResponse, killedIDs: string | string[], inputKey: string, runKind: "baseline" | "fault" = "fault") => {
      const killed = new Set(Array.isArray(killedIDs) ? killedIDs : [killedIDs]);
      stopAnimation();
      const severed = new Set(sim.severed_paths ?? []);
      const order = sim.cascade ?? [];
      setSimSummary(sim);
      setSimInputKey(inputKey);
      setSimRunKind(runKind);

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
      applySimResult(sim, selectedNodeID, JSON.stringify([doc, workload]));
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
      applySimResult(sim, [], JSON.stringify([doc, workload]), "baseline");
      return sim;
    } catch (err) {
      setSimError(describeSimError(err));
      return null;
    } finally {
      setSimBusy(false);
    }
  }, [nodes, edges, workloadForm, applySimResult]);

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
        applySimResult(sim, killedTargets(faults), JSON.stringify([doc, workload]));
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

  const renameNode = useCallback((nodeID: string, label: string) => {
    setNodes((nds) => nds.map((n) => n.id === nodeID ? { ...n, data: { ...n.data, label } } : n));
  }, [setNodes]);

  const attachResources = useCallback((nodeID: string, spec: ConfigurationSpec, resourceIDs: string[]) => {
    setEdges((eds) => replaceAttachment(nodeID, spec, resourceIDs, nodes, eds));
  }, [nodes, setEdges]);

  const createResource = useCallback((nodeID: string, spec: ConfigurationSpec, label: string): string => {
    const owner = nodes.find((n) => n.id === nodeID);
    const id = freshID("config");
    const size = CONTAINER_SIZE[spec.serviceID];
    const rank = containerRank(spec.serviceID);
    const resource: Node<CanvasNodeData> = {
      id, type: "golden",
      position: { x: (owner?.position.x ?? 0) + 320, y: owner?.position.y ?? 0 },
      ...(size ? { style: { width: size.width, height: size.height }, zIndex: -(4 - rank) } : {}),
      data: { nodeType: spec.nodeType, serviceID: spec.serviceID, label, capability: {} },
    };
    const nextNodes = [...nodes, resource];
    const assigned = attachedResources(nodeID, spec, nodes, edges).map((n) => n.id);
    setNodes(nextNodes);
    setEdges((eds) => replaceAttachment(nodeID, spec, spec.multiple ? [...assigned, id] : [id], nextNodes, eds));
    return id;
  }, [nodes, edges, setNodes, setEdges]);

  const arrangeCanvas = useCallback(() => {
    setNodes((nds) => arrangeServiceView(nds, edges));
    setLayoutVersion((v) => v + 1);
  }, [edges, setNodes]);

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
        const { style: _style, zIndex: _z, width: _width, height: _height, measured: _measured, ...rest } = n;
        void _style;
        void _z;
        void _width;
        void _height;
        void _measured;
        return {
          ...rest,
          ...(rank > 0 && size ? { style: { width: size.width, height: size.height }, zIndex: -(4 - rank) } : {}),
          data: {
            ...n.data,
            serviceID: serviceID || undefined,
            placedOnCanvas: true,
            label: serviceID && n.data.label === (n.data.serviceID ? labelForService(n.data.serviceID) : NODE_TYPE_LABELS[n.data.nodeType])
              ? labelForService(serviceID) : n.data.label,
          },
        };
      });
      setNodes(next);
    },
    [nodes, setNodes],
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

  const onConnect = useCallback((connection: Connection) => {
    if (!caps.canEditDesign) return;
    const destination = wrapperRef.current?.querySelector(`[data-id="${CSS.escape(connection.target)}"]`)?.getBoundingClientRect();
    const existing = edges.find((edge) => edge.source === connection.source && edge.target === connection.target
      && (edge.sourceHandle ?? null) === (connection.sourceHandle ?? null) && (edge.targetHandle ?? null) === (connection.targetHandle ?? null));
    setConnectionPicker({ connection, edgeID: existing?.id,
      anchor: { x: destination?.right ?? window.innerWidth / 2, y: destination?.top ?? window.innerHeight / 2 } });
  }, [caps.canEditDesign, edges]);

  const chooseConnectionType = (edgeType: EdgeType) => {
    if (!connectionPicker || !caps.canEditDesign) return;
    const { connection, edgeID } = connectionPicker;
    setEdges((current) => edgeID
      ? current.map((edge) => edge.id === edgeID ? { ...edge, label: EDGE_TYPE_LABELS[edgeType], data: { ...edge.data!, edgeType } } : edge)
      : addEdge({ ...connection, id: freshID("connection"), label: EDGE_TYPE_LABELS[edgeType], data: { edgeType } }, current));
    setConnectionPicker(null);
  };

  const editConnection = (edge: Edge<CanvasEdgeData>, anchor: { x: number; y: number }) => {
    if (!caps.canEditDesign || !edges.some((stored) => stored.id === edge.id)) return;
    setConnectionPicker({ edgeID: edge.id, connection: { source: edge.source, target: edge.target,
      sourceHandle: edge.sourceHandle ?? null, targetHandle: edge.targetHandle ?? null }, anchor });
  };

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
          placedOnCanvas: true,
          ...(serviceID ? { serviceID } : {}),
        },
      };
      setNodes((nds) => nds.concat(newNode));
      setSelectedNodeID(id);
      setShowWorkloadForm(false);
    },
    [screenToFlowPosition, setNodes],
  );

  const onNodesDelete = useCallback((deleted: Node<CanvasNodeData>[]) => {
    const ids = new Set(deleted.map((n) => n.id));
    // React Flow only sees rendered edges; also remove hidden settings relationships.
    setEdges((eds) => eds.filter((e) => !ids.has(e.source) && !ids.has(e.target)));
    setSelectedNodeID((id) => id && ids.has(id) ? null : id);
  }, [setEdges]);

  const doc = serialize(nodes, edges);
  const journeys = useMemo(() => buildWorkload(workloadForm).journeys ?? [], [workloadForm]);

  // Default to the first declared journey once one exists and nothing is selected
  // yet — a convenience only; it never invents a selection when the architect has
  // declared no journeys at all.
  useEffect(() => {
    if (!journeys.some((j) => j.id === selectedJourneyID)) {
      setSelectedJourneyID(journeys[0]?.id ?? null);
    }
  }, [journeys, selectedJourneyID]);

  // selectedFlow/journeyNodeIDs/journeyHopKeys (PC-127) project the selected
  // journey's own Hops (from /simulate's flow_detail — never recomputed) onto the
  // current canvas: which node IDs and which from/to edge pairs to highlight. Purely
  // a lookup against server-returned hop pairs, not a flow computation of its own.
  const currentSimulation = simInputKey === simulationInputKey ? simSummary : null;
  const selectedFlow = currentSimulation?.flow_detail.find((f) => f.JourneyID === selectedJourneyID) ?? null;
  const trafficVisible = trafficEnabled && caps.showsJourneyPanel;
  const declaredDns = dnsLookupForJourney(doc.nodes, journeys.find((j) => j.id === selectedJourneyID));
  const dnsNodeID = declaredDns?.To;
  const dnsAliasID = declaredDns?.ResolvesTo;
  const dnsLookup = useMemo(() => dnsNodeID && dnsAliasID ? { From: "internet" as const, To: dnsNodeID, ResolvesTo: dnsAliasID } : null, [dnsNodeID, dnsAliasID]);
  const architectureDns = dnsLookupForJourney(doc.nodes, journeys.find((j) => j.path[0] === "internet"));
  const traffic = useTrafficPlayback(selectedFlow, trafficVisible && !simBusy, dnsLookup);
  const labelForNode = (id: string) => id === "internet" ? "Internet clients" : nodes.find((n) => n.id === id)?.data.label ?? id;
  const selectTrafficJourney = (id: string) => { traffic.pause(); setSelectedJourneyID(id); };
  const followTraffic = async () => {
    setMode("simulate");
    setTrafficEnabled(true);
    setShowJourneyPanel(false);
    const journeyID = journeys.find((j) => j.id === selectedJourneyID)?.id ?? journeys[0]?.id;
    setSelectedJourneyID(journeyID ?? null);
    const flow = mode === "design" || !selectedFlow
      ? (await runBaseline())?.flow_detail.find((f) => f.JourneyID === journeyID) ?? null
      : selectedFlow;
    traffic.start(flow, true);
    setLayoutVersion((v) => v + 1);
  };
  const journeyNodeIDs = useMemo(() => {
    const ids = new Set<string>();
    for (const hop of selectedFlow?.Hops ?? []) {
      ids.add(hop.From);
      ids.add(hop.To);
    }
    if (selectedFlow && dnsLookup) { ids.add(dnsLookup.From); ids.add(dnsLookup.To); }
    return ids;
  }, [selectedFlow, dnsLookup]);
  const journeyHopKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const hop of selectedFlow?.Hops ?? []) {
      keys.add(trafficHopKey(hop.From, hop.To));
    }
    if (selectedFlow && dnsLookup) keys.add(trafficHopKey(dnsLookup.From, dnsLookup.To));
    return keys;
  }, [selectedFlow, dnsLookup]);
  const loadByNodeID = useMemo(() => {
    const loadEntries = currentSimulation?.load ?? [];
    const m = new Map<string, (typeof loadEntries)[number]>();
    for (const l of loadEntries) m.set(l.NodeID, l);
    return m;
  }, [currentSimulation]);

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
  const authoredVisibleNodes = visibleServiceNodes(nodes, false);
  const internetHop = trafficVisible ? selectedFlow?.Hops.find((h) => h.From === "internet") : undefined;
  const internetTarget = authoredVisibleNodes.find((n) => n.id === internetHop?.To);
  const dnsTarget = authoredVisibleNodes.find((n) => n.id === architectureDns?.To);
  // The client represents the workload's internet sentinel. Saved DNS intent also
  // makes it visible in Design; it is never submitted as a cloud resource.
  const internetDisplayID = "traffic-display:internet";
  const clientAnchor = dnsTarget ?? internetTarget;
  const visibleNodes: Node<CanvasNodeData>[] = clientAnchor ? [...authoredVisibleNodes, {
    id: internetDisplayID, type: "golden", position: { x: clientAnchor.position.x - 320, y: clientAnchor.position.y },
    data: { nodeType: "external_dependency", label: "Internet clients", capability: {}, trafficExternal: true },
    width: 220, height: 64, measured: { width: 220, height: 64 },
    selectable: false, draggable: false, connectable: false, deletable: false, style: { pointerEvents: "none" },
  }] : authoredVisibleNodes;
  const visibleIDs = new Set(visibleNodes.map((n) => n.id));
  const displayID = (id: string) => id === "internet" ? internetDisplayID : id;
  const activeEndpoints = traffic.active?.dns ? [traffic.active.dns.From, traffic.active.dns.To] : traffic.active?.hops.flatMap((h) => [h.From, h.To]) ?? [];
  const activeNodeIDs = new Set(activeEndpoints.map(displayID));
  const displayNodes = [
    ...visibleNodes.map((n) => {
      const load = loadByNodeID.get(n.id);
      return {
        ...n,
        data: {
          ...n.data,
          canResizeContainer: caps.canEditDesign && containerRank(n.data.serviceID) > 0,
          journeyOnPath: journeyNodeIDs.has(n.data.trafficExternal ? "internet" : n.id),
          trafficStep: trafficVisible && activeNodeIDs.has(n.id) ? traffic.index + 1 : undefined,
          trafficDimmed: trafficVisible && !!selectedFlow && !journeyNodeIDs.has(n.data.trafficExternal ? "internet" : n.id),
          utilization: load ? load.Utilization : undefined,
          notAssessableLoad: load ? load.Capacity === null : false,
          subnetFact: factByNode.get(n.id),
          configurationSummary: configurationSummary(n, nodes, edges),
        },
      };
    }),
  ] as unknown as Node<CanvasNodeData>[];

  // displayEdges applies severed styling at render time only — edges' own stored
  // `data.severed` (set by applySimResult) never becomes a React Flow `style`/
  // `animated` prop directly, so serialize.ts's own field whitelist stays the single
  // source of truth for what's UI-only vs. wire-shape.
  const visibleEdges = edges.filter((e) => visibleIDs.has(e.source) && visibleIDs.has(e.target));
  const supplementalEdges: Edge<CanvasEdgeData>[] = [];
  if (dnsTarget && architectureDns) {
    supplementalEdges.push({ id: "traffic-display:dns-lookup", source: internetDisplayID, target: dnsTarget.id,
      sourceHandle: "dns-query", targetHandle: "dns-query", data: { edgeType: "routes_to" }, label: "DNS lookup", selectable: false, deletable: false });
    if (visibleIDs.has(architectureDns.ResolvesTo)) supplementalEdges.push({ id: "traffic-display:https-request", source: internetDisplayID, target: architectureDns.ResolvesTo,
      data: { edgeType: "routes_to" }, label: "HTTPS request", selectable: false, deletable: false });
  }
  if (trafficVisible) {
    for (const hop of selectedFlow?.Hops ?? []) {
      const source = displayID(hop.From), target = displayID(hop.To);
      if (!visibleIDs.has(source) || !visibleIDs.has(target) || [...visibleEdges, ...supplementalEdges].some((e) => e.source === source && e.target === target)) continue;
      supplementalEdges.push({ id: `traffic-display:${trafficHopKey(source, target)}`, source, target,
        data: { edgeType: "routes_to" }, label: "Traffic hop", selectable: false, deletable: false });
    }
  }
  const displayEdges = [...visibleEdges, ...supplementalEdges].map((edge) => {
    const from = edge.source === internetDisplayID ? "internet" : edge.source;
    const key = trafficHopKey(from, edge.target);
    const onPath = journeyHopKeys.has(key);
    const activeHop = trafficVisible ? traffic.active?.hops.find((h) => trafficHopKey(h.From, h.To) === key) : undefined;
    const activeDns = trafficVisible && traffic.active?.dns && trafficHopKey(traffic.active.dns.From, traffic.active.dns.To) === key;
    const dnsEdge = edge.id === "traffic-display:dns-lookup";
    const aliasEdge = nodes.find((n) => n.id === edge.source)?.data.capability.design_alias === edge.target;
    const hop = activeHop ?? selectedFlow?.Hops.findLast((h) => trafficHopKey(h.From, h.To) === key);
    const step = traffic.steps.findIndex((s) => s.dns ? trafficHopKey(s.dns.From, s.dns.To) === key : s.hops.some((h) => trafficHopKey(h.From, h.To) === key));
    const label = aliasEdge ? "Resolves to ALB address" : edge.id.startsWith("traffic-display:") ? edge.label : edge.data?.edgeType ? EDGE_TYPE_LABELS[edge.data.edgeType] : edge.label;
    const color = edge.data?.severed ? "var(--danger-ink)" : dnsEdge ? "var(--cyan-ink)" : onPath && hop && !hop.Allowed ? "var(--warning-ink)" : onPath ? "var(--purple-ink)" : "var(--subtle)";
    return { ...edge, type: "traffic", animated: false,
      label: trafficVisible && onPath ? `${activeHop || activeDns ? traffic.index + 1 : step + 1}. ${label}` : label,
      labelStyle: { fontSize: 10, fontWeight: 500, fill: "var(--ink-secondary)" }, labelBgStyle: { fill: "var(--surface)", stroke: "var(--line)", strokeWidth: 1 }, labelBgBorderRadius: 6, labelBgPadding: [9, 5] as [number, number],
      markerEnd: { type: MarkerType.ArrowClosed, color, width: 18, height: 18 },
      style: { stroke: color, strokeWidth: activeHop || activeDns ? 3.5 : onPath || edge.selected ? 2.5 : 1.7,
        strokeDasharray: dnsEdge || aliasEdge || edge.data?.severed || onPath && hop && !hop.Allowed ? "6 4" : undefined,
        opacity: trafficVisible && !onPath && !edge.data?.severed ? 0.25 : 1 },
      data: { ...edge.data!, traffic: activeHop || activeDns ? { active: true, allowed: activeHop?.Allowed, kind: activeDns ? "dns" as const : undefined, playing: traffic.playing,
        reducedMotion: traffic.reducedMotion, duration: traffic.duration, packetKey: traffic.packetKey } : undefined } };
  });

  return (
    <div className="app-shell">
      <a href="#main-content" className="skip-link">Skip to workspace</a>
      <aside className="app-navigation" aria-label="Workspace navigation">
        <a className="workspace-brand" href="#" aria-label="Preflight home" onClick={(event) => { event.preventDefault(); traffic.pause(); setMode("design"); }}>
          <span className="brand-mark" aria-hidden><svg width="23" height="23" viewBox="0 0 24 24" fill="none"><path d="M5 18V6h7a4 4 0 010 8H9" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/><path d="M15 16l3 3 4-5" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"/></svg></span><span>Preflight</span>
        </a>
        <div className="navigation-label">Architect workspace</div>
        <nav data-testid="mode-bar" aria-label="Workspace modes">
          {MODES.map((m) => (
            <button key={m.id} data-mode={m.id} aria-label={m.label} title={m.label} onClick={() => { traffic.pause(); setMode(m.id); }}
              aria-current={mode === m.id ? "page" : undefined}
              className={`mode-button${mode === m.id ? " active" : ""}`}>
              <WorkspaceIcon mode={m.id} /><span>{m.label}</span><span className="navigation-tooltip" aria-hidden>{m.label}</span>
            </button>
          ))}
        </nav>
        <div className="navigation-footer"><ThemeToggle /><span className="rail-avatar" title="Local workspace">PF</span></div>
      </aside>
      <main className="workspace-main" id="main-content" tabIndex={-1}>
        <header className="workspace-header">
          <div className="workspace-breadcrumb"><span className="header-product">Preflight</span><span aria-hidden>/</span><strong>Architecture workspace</strong></div><span className="header-mode">{MODES.find((m) => m.id === mode)?.label}</span>
          <span className="session-pill" data-testid="session-label">session {sessionIDRef.current.slice(0, 8)} · {latestVersion === null ? "no version assessed yet" : `latest v${latestVersion}`}</span>
        </header>
      {mode === "report" ? (
        <ReportView sessionID={sessionIDRef.current} onReprice={repriceCurrentDesign} />
      ) : mode === "analyze" ? (
        <AnalyzeView sessionID={sessionIDRef.current} latestVersion={latestVersion} />
      ) : (
    <>
      <div className="canvas-toolbar" role="toolbar" aria-label="Architecture tools">
        <div className="toolbar-main-row">
          <div className="canvas-title"><WorkspaceIcon mode={mode} /><span>Service canvas</span></div>
          {caps.showsAuthoringPanels && <button className="library-toggle" onClick={() => setShowLibrary((value) => !value)} aria-pressed={showLibrary} title="Show or hide the service library">Library</button>}
          {caps.canEditDesign && (
            <>
              <div className="toolbar-actions" aria-label="Canvas panels">
                <button
                  onClick={() => { if (!showWorkloadForm) setSelectedNodeID(null); setShowWorkloadForm((v) => !v); }}
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
          <button onClick={() => void followTraffic()} disabled={simBusy || journeys.length === 0} data-testid="follow-traffic" title={journeys.length ? "Follow a journey hop by hop using the simulation results" : "Add a journey in Workload to follow traffic"}>Follow traffic <span aria-hidden>→</span></button>
          {caps.canEditDesign && <button onClick={clearArchitecture} disabled={templateLoading || simBusy || nodes.length === 0} data-testid="clear-architecture" title="Clear all services, connections and template workload">Clear design</button>}
          {caps.canEditDesign && <button onClick={arrangeCanvas} data-testid="arrange-canvas" title="Arrange services in the direction of traffic">Arrange</button>}
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
      {trafficVisible && <TrafficPlayer journeys={journeys} selectedJourneyID={selectedJourneyID} onSelectJourney={selectTrafficJourney}
        flow={selectedFlow} playback={traffic} labelForNode={labelForNode} busy={simBusy}
        onFocus={() => void fitCanvas()}
        onClose={() => { traffic.pause(); setTrafficEnabled(false); }} />}
    <div className="workspace">
      {caps.showsAuthoringPanels && showLibrary && <Palette>
        <div className="library-template">              <label className="toolbar-field template-field">
                <span>Start from a template</span>
                <select
                  value=""
                  aria-label="Architecture template"
                  disabled={templatesFetching || templateLoading || templateList.length === 0}
                  onChange={(e) => requestTemplate(e.target.value)}
                  data-testid="template-picker"
                >
                  <option value="">{templatesFetching ? "Loading templates…" : templateLoading ? "Opening architecture…" : templateList.length ? "Choose an architecture…" : "Templates unavailable"}</option>
                  {templateList.map((t) => <option key={t.id} value={t.id} title={t.description}>{t.name}</option>)}
                </select>
              </label>
</div>
        {templateListError && <div className="template-load-error" role="alert"><p>{templateListError}</p><button onClick={() => { setTemplatesFetching(true); setTemplateListError(null); setTemplateRetry((value) => value + 1); }}>Retry templates</button></div>}
        {loadedTemplate && <details className="template-context" data-testid="loaded-template"><summary>{loadedTemplate.name}<span>About this template</span></summary><p>{loadedTemplate.description}</p></details>}
      </Palette>}
      <div className="canvas-column">
        {simError ? <div className="status-strip mode-error" role="alert">Simulation could not run: {simError}</div>
          : currentSimulation && <SimulationStatus result={currentSimulation} runKind={simRunKind} labelForNode={labelForNode} animationStep={animStep}
            onSetCapacity={() => { traffic.pause(); setMode("design"); setSelectedNodeID(null); setShowWorkloadForm(true); }} />}
        <div className="canvas-stage" data-editable={caps.canEditDesign}
          onKeyDownCapture={(event) => {
            if (event.key !== "Enter" || !caps.canEditDesign || !(event.target instanceof Element)) return;
            const element = event.target.closest(".react-flow__edge");
            const edge = edges.find((item) => item.id === element?.getAttribute("data-id"));
            if (!element || !edge) return;
            event.preventDefault(); event.stopPropagation();
            const bounds = element.getBoundingClientRect();
            editConnection(edge, { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 });
          }} ref={wrapperRef} onDragOver={caps.canEditDesign ? onDragOver : undefined} onDrop={caps.canEditDesign ? onDrop : undefined}>
          <ReactFlow
            nodes={displayNodes}
            edges={connectionPicker && !connectionPicker.edgeID ? [...displayEdges, {
              ...connectionPicker.connection, id: "connection-preview", type: "traffic", label: "Choose type",
              selectable: false, deletable: false, style: { stroke: "var(--accent)", strokeWidth: 2, strokeDasharray: "5 4" },
            }] : displayEdges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onEdgeClick={(event, edge) => {
              const bounds = event.currentTarget.getBoundingClientRect();
              editConnection(edge, { x: event.clientX || bounds.x + bounds.width / 2, y: event.clientY || bounds.y + bounds.height / 2 });
            }}
            onNodeClick={onNodeClick}
            onNodesDelete={onNodesDelete}
            onPaneClick={() => setSelectedNodeID(null)}
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
            fitViewOptions={{ padding: 0.18, maxZoom: 1 }}
          >
            <Background gap={12} size={1} color="var(--line-strong)" />
            <Controls />
          </ReactFlow>
          {nodes.length === 0 && caps.canEditDesign && <div className="canvas-empty">
            <div className="empty-diagram" aria-hidden><span /><i /><span /><i /><span /></div>
            <span className="eyebrow">FROM IDEA TO ARCHITECTURE</span>
            <h1>Your system starts here.</h1>
            <p>Bring your services together, connect the flow,<br />then put your design to the test.</p>
            <button onClick={() => setShowLibrary(true)}>Explore the service library <span aria-hidden>↗</span></button>
            <small>Or choose a starting architecture from the library.</small>
          </div>}
          {nodes.length > 0 && caps.canEditDesign && (
            <div className="canvas-view-hint">Service view <span>Click a service to configure access and network settings</span></div>
          )}
        <div className="toolbar-context-row">
          {caps.canEditDesign ? (
            <span id="connection-guide">
              <span className="connection-guide-arrow" aria-hidden>↗</span>
              Drag between service ports, then choose a connection type. Click a connection to edit it.
            </span>
          ) : journeys.length === 0 && caps.canRunBaseline ? (
            <span>Add a journey in Design → Workload to run a baseline.</span>
          ) : (
            <span>Select a service on the canvas to inspect a failure.</span>
          )}
          <span className="canvas-count">{authoredVisibleNodes.length} services · {visibleEdges.length} connections{nodes.length > authoredVisibleNodes.length ? ` · ${nodes.length - authoredVisibleNodes.length} settings` : ""}</span>
        </div>
          {showJSON && (
            <pre
              style={{
                position: "absolute",
                right: 8,
                top: 8,
                maxWidth: 420,
                maxHeight: "80%",
                overflow: "auto",
                background: "var(--ink)",
                color: "var(--line)",
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
      {caps.showsAuthoringPanels && showWorkloadForm && (
        <div className="workspace-panel workload-drawer">
          <div className="drawer-heading"><div><span className="eyebrow">DESIGN INPUTS</span><h2>Workload requirements</h2></div><button onClick={() => setShowWorkloadForm(false)} aria-label="Close workload">×</button></div>
          <WorkloadForm
            value={workloadForm}
            onChange={setWorkloadForm}
            nodeLabels={Object.fromEntries(nodes.map((n) => [n.id, n.data.label]))}
            pickingPathForRow={pickingJourneyRowIndex}
            onStartPickPath={startPickPath}
            onStopPickPath={stopPickPath}
          />
        </div>
      )}
      {selectedNode && caps.showsAuthoringPanels && (
        <Inspector
          node={selectedNode}
          onClose={() => setSelectedNodeID(null)}
          nodes={nodes.map((n) => ({ ...n, data: { ...n.data, subnetFact: factByNode.get(n.id) } }))}
          edges={edges}
          onRename={renameNode}
          onAttach={attachResources}
          onCreateResource={createResource}
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
        <div style={{ width: 340, borderLeft: "1px solid var(--line)", overflowY: "auto" }}>
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
        <div style={{ width: 320, borderLeft: "1px solid var(--line)", overflowY: "auto" }}>
          <JourneyPanel
            journeys={journeys}
            flowDetail={currentSimulation?.flow_detail ?? []}
            latency={currentSimulation?.latency}
            load={currentSimulation?.load ?? []}
            selectedJourneyID={selectedJourneyID}
            onSelectJourney={selectTrafficJourney}
          />
        </div>
      )}
    </div>
    </>
      )}
      </main>
      {pendingTemplate && <TemplateConfirmation name={pendingTemplate.name}
        onCancel={() => setPendingTemplate(null)}
        onProceed={() => { const id = pendingTemplate.id; setPendingTemplate(null); void loadTemplate(id); }} />}
      {connectionPicker && caps.canEditDesign && <ConnectionPicker
        source={labelForNode(connectionPicker.connection.source)} target={labelForNode(connectionPicker.connection.target)}
        currentType={connectionPicker.edgeID ? edges.find((edge) => edge.id === connectionPicker.edgeID)?.data?.edgeType : undefined}
        anchor={connectionPicker.anchor} onChoose={chooseConnectionType} onClose={() => setConnectionPicker(null)} />}

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

function WorkspaceIcon({ mode }: { mode: Mode }) {
  const paths: Record<Mode, string> = {
    design: "M3 3h6v6H3z M15 3h6v6h-6z M3 15h6v6H3z M15 15h6v6h-6z M9 6h6 M6 9v6 M18 9v6 M9 18h6",
    simulate: "M8 4l12 8-12 8z",
    failure_lab: "M9 3h6 M10 3v6L4 19a1 1 0 001 2h14a1 1 0 001-2L14 9V3 M7 15h10",
    analyze: "M4 20V12 M10 20V4 M16 20V9 M22 20H2",
    report: "M14 3H5v18h14V8z M14 3v5h5 M8 12h8 M8 16h6",
  };
  return <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[mode]} /></svg>;
}
