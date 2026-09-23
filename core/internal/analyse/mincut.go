// This file is the tracked cost of ADR-004: NetworkX's built-in min-cut has no Go
// equivalent, so PC-14 owns implementing one by hand (Stoer-Wagner global cut, or a
// max-flow-derived cut per s-t pair — ADR-004 leaves the choice open). MinVertexCut
// below is a max-flow-derived MINIMUM VERTEX CUT, chosen deliberately over a global
// edge cut: an AWS SPOF is almost always a single failed RESOURCE (a NAT gateway, a
// database instance) — a vertex — not a single failed connection between two resources
// that individually survive. A vertex cut is the structurally correct question to ask.
//
// Implemented and hand-verified in isolation (mincut_test.go, against small graphs
// with a hand-computed known answer) BEFORE this is wired into any SPOF detector that
// consumes real core.IR data — a wrong min-cut inside a full simulation trace is a
// debugging problem; a wrong min-cut against a 5-node graph with a known answer is a
// five-minute fix.
package analyse

import "sort"

// MinVertexCutResult is MinVertexCut's output. Three distinct outcomes, not two —
// found by testing this exact conflation against a 2-node graph before it ever reached
// real IR data:
//
//   - Size == 0, Uncuttable == false: source cannot reach sink at all. Vacuously safe,
//     not a SPOF — there is nothing to protect, no dependency exists.
//   - Uncuttable == true: source reaches sink via a path with ZERO intermediate
//     vertices (a direct edge) — no finite number of vertex removals can ever
//     disconnect them. This is not "safe" and not "one cuttable point" — it is a
//     distinct outcome a caller must not silently collapse into either.
//   - Size > 0, Uncuttable == false: the ordinary case — CutVertices lists the actual
//     minimum vertex cut.
type MinVertexCutResult struct {
	Size        int
	CutVertices []string
	Uncuttable  bool
}

// MinVertexCut computes the minimum number of vertices (excluding source and sink
// themselves, which are never reported as their own cut) whose removal disconnects
// every directed path from source to sink.
//
// Technique: the standard vertex-splitting reduction to max-flow / min-EDGE-cut
// (Even, 1975 — a well-established technique, not invented here). Every vertex v
// (other than source/sink) is split into v_in -> v_out with capacity 1; every original
// edge u->v becomes u_out -> v_in with effectively-infinite capacity. Max-flow from
// source to sink then equals the min vertex cut (max-flow min-cut theorem), and the
// actual cut vertices are found by a residual-graph reachability pass after the flow
// is computed.
//
// Determinism (I1): BFS augmenting paths (Edmonds-Karp) always explore neighbors in
// the same sorted order (see adjacency), so the same graph always produces the same
// flow decomposition and therefore the same reported CutVertices — even when several
// distinct minimum cuts exist and any one of them would be an equally valid answer.
// This satisfies "reproducible across runs" (ADR-004/PC-14's own stated requirement);
// it does not claim to always return some externally-defined canonical cut when ties
// exist, only ever the same one, run after run.
func MinVertexCut(edges []DirectedEdge, source, sink string) MinVertexCutResult {
	if source == sink {
		return MinVertexCutResult{} // a node is never its own SPOF
	}
	for _, e := range edges {
		if e.From == source && e.To == sink {
			// A direct edge has zero intermediate vertices — nothing for the
			// vertex-splitting reduction below to bound flow by, so running it would
			// report a bogus finite Size derived from the algorithm's internal
			// "infinite" capacity constant rather than a meaningful answer. See the
			// type doc comment: this is its own outcome, not Size 0 and not a normal
			// cut.
			return MinVertexCutResult{Uncuttable: true}
		}
	}

	g := newSplitFlowGraph(edges, source, sink)
	flowValue := 0
	for {
		path := g.findAugmentingPath(source, sink)
		if path == nil {
			break
		}
		bottleneck := g.pathBottleneck(path)
		g.applyFlow(path, bottleneck)
		flowValue += bottleneck
	}

	reachable := g.residualReachableSet(source)
	var cut []string
	for v := range g.splitOf {
		if v == source || v == sink {
			continue
		}
		vIn, vOut := g.splitOf[v][0], g.splitOf[v][1]
		if reachable[vIn] && !reachable[vOut] {
			cut = append(cut, v)
		}
	}
	sort.Strings(cut)

	return MinVertexCutResult{Size: flowValue, CutVertices: cut}
}

// infiniteCapacity stands in for "infinite" on the split edges between different
// vertices' in/out halves — it only needs to exceed any possible vertex-capacity path
// (each vertex contributes at most 1), so the total vertex count is already a safe,
// exact-enough bound; a literal huge constant is avoided so overflow is structurally
// impossible regardless of graph size.
func infiniteCapacity(vertexCount int) int { return vertexCount + 1 }

// splitFlowGraph is the vertex-split residual graph MinVertexCut operates on. Nodes
// are named strings: for an original vertex v, "v#in" and "v#out" (source and sink are
// NOT split — see newSplitFlowGraph). Edges are stored as a residual capacity map,
// mutated in place as flow is pushed (the standard residual-graph technique: pushing
// flow on u->v reduces cap[u][v] and increases cap[v][u] by the same amount, which is
// what lets augmenting paths "cancel" earlier flow along a reverse edge if needed).
type splitFlowGraph struct {
	cap     map[string]map[string]int
	adj     map[string][]string  // fixed adjacency for deterministic BFS order — see below
	splitOf map[string][2]string // original vertex -> [v#in, v#out]; source/sink map to themselves
}

func splitName(v string, half string) string { return v + "#" + half }

func newSplitFlowGraph(edges []DirectedEdge, source, sink string) *splitFlowGraph {
	vertices := vertexSet(edges)
	g := &splitFlowGraph{
		cap:     map[string]map[string]int{},
		adj:     map[string][]string{},
		splitOf: map[string][2]string{},
	}
	addCap := func(from, to string, c int) {
		if g.cap[from] == nil {
			g.cap[from] = map[string]int{}
		}
		g.cap[from][to] += c
		if g.cap[to] == nil {
			g.cap[to] = map[string]int{}
		}
		if _, ok := g.cap[to][from]; !ok {
			g.cap[to][from] = 0 // ensure the reverse residual edge exists, at capacity 0
		}
		g.adj[from] = appendUnique(g.adj[from], to)
		g.adj[to] = appendUnique(g.adj[to], from) // residual graph needs both directions walkable
	}

	inf := infiniteCapacity(len(vertices))

	for _, v := range vertices {
		if v == source || v == sink {
			g.splitOf[v] = [2]string{v, v}
			continue
		}
		vIn, vOut := splitName(v, "in"), splitName(v, "out")
		g.splitOf[v] = [2]string{vIn, vOut}
		addCap(vIn, vOut, 1) // the vertex's own capacity: it can be "used" once
	}

	nodeOut := func(v string) string {
		if v == source || v == sink {
			return v
		}
		return g.splitOf[v][1]
	}
	nodeIn := func(v string) string {
		if v == source || v == sink {
			return v
		}
		return g.splitOf[v][0]
	}

	for _, e := range edges {
		addCap(nodeOut(e.From), nodeIn(e.To), inf)
	}

	sortAdjacency(g.adj)
	return g
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

func sortAdjacency(adj map[string][]string) {
	for v := range adj {
		sort.Strings(adj[v])
	}
}

// findAugmentingPath runs BFS from source to sink over edges with remaining residual
// capacity > 0, exploring each node's neighbors in sorted order (via g.adj, built
// once and never re-sorted mid-algorithm) so the path found is a deterministic
// function of the graph alone.
func (g *splitFlowGraph) findAugmentingPath(source, sink string) []string {
	visited := map[string]bool{source: true}
	parent := map[string]string{}
	queue := []string{source}

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		if u == sink {
			return reconstructPath(parent, source, sink)
		}
		for _, v := range g.adj[u] {
			if visited[v] || g.cap[u][v] <= 0 {
				continue
			}
			visited[v] = true
			parent[v] = u
			queue = append(queue, v)
		}
	}
	return nil
}

func reconstructPath(parent map[string]string, source, sink string) []string {
	path := []string{sink}
	for cur := sink; cur != source; {
		prev := parent[cur]
		path = append(path, prev)
		cur = prev
	}
	// reverse into source->...->sink order
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func (g *splitFlowGraph) pathBottleneck(path []string) int {
	min := -1
	for i := 0; i+1 < len(path); i++ {
		c := g.cap[path[i]][path[i+1]]
		if min == -1 || c < min {
			min = c
		}
	}
	return min
}

func (g *splitFlowGraph) applyFlow(path []string, amount int) {
	for i := 0; i+1 < len(path); i++ {
		g.cap[path[i]][path[i+1]] -= amount
		g.cap[path[i+1]][path[i]] += amount
	}
}

// residualReachableSet is the standard "find the min cut after max-flow" step: BFS
// from source over whatever residual capacity remains. Everything reachable is on the
// source side of the cut; everything not reachable is on the sink side.
func (g *splitFlowGraph) residualReachableSet(source string) map[string]bool {
	reachable := map[string]bool{source: true}
	queue := []string{source}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range g.adj[u] {
			if reachable[v] || g.cap[u][v] <= 0 {
				continue
			}
			reachable[v] = true
			queue = append(queue, v)
		}
	}
	return reachable
}
