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
  DEFAULT_EDGE_TYPE,
  type NodeType,
  type EdgeType,
} from "./goldenVocabulary";
import type { CanvasNodeData, CanvasEdgeData } from "./types";
import { serialize } from "./serialize";
import { GoldenNode } from "./GoldenNode";
import { Inspector } from "./Inspector";
import { assessCanvas, simulateNodeLoss, simulateBaseline, describeSimError, type SimulateResponse } from "./api";
import { ReportView } from "./ReportView";
import { WorkloadForm, buildWorkload, emptyWorkloadFormValue, type WorkloadFormValue } from "./WorkloadForm";
import { JourneyPanel } from "./JourneyPanel";

const nodeTypes = { golden: GoldenNode };

let nextID = 1;
function freshID(prefix: string): string {
  return `${prefix}-${nextID++}`;
}

function Palette() {
  const onDragStart = (event: React.DragEvent, nodeType: NodeType) => {
    event.dataTransfer.setData("application/preflight-node-type", nodeType);
    event.dataTransfer.effectAllowed = "move";
  };

  return (
    <aside style={{ width: 220, borderRight: "1px solid #e2e8f0", padding: 12, overflowY: "auto" }}>
      <h2 style={{ fontSize: 14, margin: "0 0 8px" }}>Golden vocabulary</h2>
      <p style={{ fontSize: 11, color: "#64748b", margin: "0 0 12px" }}>
        Drag a type onto the canvas. This is the complete list — no other node types
        exist here, by design (PC-85).
      </p>
      {NODE_TYPES.map((nt) => (
        <div
          key={nt}
          draggable
          onDragStart={(e) => onDragStart(e, nt)}
          style={{
            padding: "6px 10px",
            marginBottom: 6,
            borderRadius: 4,
            border: "1px solid #cbd5e1",
            background: "#f8fafc",
            cursor: "grab",
            fontSize: 12,
          }}
        >
          {NODE_TYPE_LABELS[nt]}
        </div>
      ))}
    </aside>
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
  // mode (PC-123): Design is this file's own pre-existing canvas; Report is a thin
  // view over PC-120/122's own already-computed results — switching modes changes
  // nothing about the canvas state underneath it.
  const [mode, setMode] = useState<"design" | "report">("design");
  const [selectedNodeID, setSelectedNodeID] = useState<string | null>(null);
  const [simBusy, setSimBusy] = useState(false);
  const [simError, setSimError] = useState<string | null>(null);
  const [simSummary, setSimSummary] = useState<SimulateResponse | null>(null);
  // selectedJourneyID (PC-127): which declared journey the JourneyPanel/canvas
  // highlight currently reflects — independent of selectedNodeID (killing a node and
  // inspecting a journey's flow are separate concerns).
  const [selectedJourneyID, setSelectedJourneyID] = useState<string | null>(null);
  const [showJourneyPanel, setShowJourneyPanel] = useState(true);

  const onNodeClick = useCallback<NodeMouseHandler>((_event, node) => {
    setSelectedNodeID(node.id);
  }, []);

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
    (sim: SimulateResponse, killedID: string) => {
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
            if (n.id === killedID) simState = "killed";
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
      const sim = await simulateBaseline(sessionIDRef.current, assessed.version_number);
      stopAnimation();
      setSimSummary(sim);
    } catch (err) {
      setSimError(describeSimError(err));
    } finally {
      setSimBusy(false);
    }
  }, [nodes, edges, workloadForm, stopAnimation]);

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
      if (!nodeType) return;

      const position = screenToFlowPosition({ x: event.clientX, y: event.clientY });
      const id = freshID(nodeType);
      const newNode: Node<CanvasNodeData> = {
        id,
        type: "golden",
        position,
        data: { nodeType, label: NODE_TYPE_LABELS[nodeType], capability: {} },
      };
      setNodes((nds) => nds.concat(newNode));
    },
    [screenToFlowPosition, setNodes],
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

  // displayNodes overlays journey highlight + utilization presentation at render
  // time only, same "never written back into node state" discipline displayEdges
  // below already applies to severed.
  const displayNodes = nodes.map((n) => {
    const load = loadByNodeID.get(n.id);
    return {
      ...n,
      data: {
        ...n.data,
        journeyOnPath: journeyNodeIDs.has(n.id),
        utilization: load ? load.Utilization : undefined,
        notAssessableLoad: load ? load.Capacity === null : false,
      },
    };
  });

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
    <div style={{ display: "flex", flexDirection: "column", height: "100vh", width: "100%" }}>
      <div style={{ display: "flex", gap: 6, padding: "6px 12px", borderBottom: "1px solid #cbd5e1", background: "#f8fafc" }}>
        <button
          onClick={() => setMode("design")}
          style={{ fontSize: 12, fontWeight: mode === "design" ? 700 : 400, background: mode === "design" ? "#e0e7ff" : undefined }}
        >
          Design
        </button>
        <button
          onClick={() => setMode("report")}
          style={{ fontSize: 12, fontWeight: mode === "report" ? 700 : 400, background: mode === "report" ? "#e0e7ff" : undefined }}
        >
          Report
        </button>
      </div>
      {mode === "report" ? (
        <ReportView sessionID={sessionIDRef.current} onReprice={repriceCurrentDesign} />
      ) : (
    <div style={{ display: "flex", flex: 1, minHeight: 0, width: "100%" }}>
      <Palette />
      {showWorkloadForm && (
        <div style={{ width: 300, borderRight: "1px solid #e2e8f0", overflowY: "auto" }}>
          <WorkloadForm value={workloadForm} onChange={setWorkloadForm} />
        </div>
      )}
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        <div style={{ padding: 8, borderBottom: "1px solid #e2e8f0", display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <label style={{ fontSize: 12 }}>
            Next edge type:{" "}
            <select
              value={pendingEdgeType}
              onChange={(e) => setPendingEdgeType(e.target.value as EdgeType)}
            >
              {EDGE_TYPES.map((et) => (
                <option key={et} value={et}>
                  {et}
                </option>
              ))}
            </select>
          </label>
          <button onClick={() => setShowJSON((v) => !v)} style={{ fontSize: 12 }}>
            {showJSON ? "Hide" : "Show"} CanvasDocument JSON
          </button>
          <span style={{ fontSize: 11, color: "#64748b" }}>
            {nodes.length} node(s), {edges.length} edge(s)
          </span>
          <span style={{ borderLeft: "1px solid #e2e8f0", height: 20 }} />
          <button onClick={() => setShowWorkloadForm((v) => !v)} style={{ fontSize: 12 }}>
            {showWorkloadForm ? "Hide" : "Show"} Workload form
          </button>
          {selectedNodeID ? (
            <span style={{ fontSize: 12 }}>
              Selected: <strong>{selectedNodeLabel ?? selectedNodeID}</strong>
            </span>
          ) : (
            <span style={{ fontSize: 12, color: "#94a3b8" }}>Click a node to select it</span>
          )}
          <button
            onClick={killSelectedNode}
            disabled={!selectedNodeID || simBusy}
            style={{ fontSize: 12, background: "#fee2e2", border: "1px solid #dc2626" }}
          >
            {simBusy ? "Simulating..." : "Kill selected node"}
          </button>
          <button onClick={clearSimulation} disabled={!simSummary} style={{ fontSize: 12 }}>
            Clear simulation
          </button>
          <span style={{ borderLeft: "1px solid #e2e8f0", height: 20 }} />
          <button onClick={runBaseline} disabled={simBusy || journeys.length === 0} style={{ fontSize: 12 }}>
            Run baseline (no fault)
          </button>
          <button onClick={() => setShowJourneyPanel((v) => !v)} style={{ fontSize: 12 }}>
            {showJourneyPanel ? "Hide" : "Show"} journey panel
          </button>
        </div>
        {(simError || simSummary) && (
          <div
            style={{
              padding: 8,
              borderBottom: "1px solid #e2e8f0",
              fontSize: 12,
              background: simError ? "#fef2f2" : "#f8fafc",
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
        <div ref={wrapperRef} style={{ flex: 1, position: "relative" }} onDragOver={onDragOver} onDrop={onDrop}>
          <ReactFlow
            nodes={displayNodes}
            edges={displayEdges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={onNodeClick}
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
      {selectedNode && <Inspector node={selectedNode} onChange={updateNodeSizing} />}
      {showJourneyPanel && (
        <div style={{ width: 320, borderLeft: "1px solid #e2e8f0", overflowY: "auto" }}>
          <JourneyPanel
            journeys={journeys}
            flowDetail={simSummary?.flow_detail ?? []}
            load={simSummary?.load ?? []}
            selectedJourneyID={selectedJourneyID}
            onSelectJourney={setSelectedJourneyID}
          />
        </div>
      )}
    </div>
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
