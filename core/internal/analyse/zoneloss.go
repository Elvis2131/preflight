// This file is PC-14's first acceptance criterion's algorithmic core: "killing a zone
// on the golden architecture correctly identifies which app nodes/NAT/etc are lost."
//
// SimulateLoss is deliberately a plain reachability computation, not min-cut: "what
// survives when these specific nodes disappear" is a different question from "what is
// the weakest link" — CLAUDE.md §14's own phrasing ("what happens when a zone...
// disappears") is fault injection (remove nodes, observe what's still reachable), not
// a search for the minimum such set. Reusing MinVertexCut's machinery here would
// answer the wrong question.
//
// Same package rule as everywhere else in this file: no dependency on preflight/core.
package analyse

import "sort"

// SimulateLoss computes which nodes become unreachable from entryPoints once killed
// nodes (and every edge touching them) are removed from the graph, compared against
// what was reachable before. Returns the sorted list of newly-lost node IDs — nodes
// that were reachable before the kill and are not reachable after.
//
// killed nodes are never themselves double-counted as "lost due to disconnection" —
// they were removed directly, which is a different fact (what was explicitly killed)
// from what merely became unreachable as a consequence.
func SimulateLoss(edges []DirectedEdge, entryPoints []string, killed map[string]bool) []string {
	before := reachableFrom(edges, entryPoints, nil)
	after := reachableFrom(edges, entryPoints, killed)

	var lost []string
	for node := range before {
		if killed[node] {
			continue // directly killed, not "lost due to disconnection"
		}
		if !after[node] {
			lost = append(lost, node)
		}
	}
	sort.Strings(lost)
	return lost
}

// CascadeOrder (PC-89) orders a set of members (typically the killed nodes unioned
// with whatever became unreachable as a consequence) by hop distance from the nearest
// killed node, walking forward along edges the same direction reachability already
// does. This is a real, structural, deterministic fact — how many dependency edges
// separate a node from the point of failure — never a timing or rate claim: PC-89's
// own explicit boundary is that failure propagation has no duration or velocity
// modeled anywhere in this codebase (Layer 2/3 remain out of scope), and hop count is
// topology, not time. Killed nodes are hop 0. A member not reached from any killed
// node by a forward edge (a disconnected fragment, or an isolated node with no
// outgoing path back to anything killed) sorts after every real hop distance, tied
// alphabetically like every other tie — never guessed at, never given a fabricated
// hop number.
func CascadeOrder(edges []DirectedEdge, killed map[string]bool, members map[string]bool) []string {
	adj := adjacency(edges)
	hop := map[string]int{}

	killedSorted := make([]string, 0, len(killed))
	for id := range killed {
		killedSorted = append(killedSorted, id)
	}
	sort.Strings(killedSorted)

	var queue []string
	for _, id := range killedSorted {
		if _, seen := hop[id]; !seen {
			hop[id] = 0
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if _, seen := hop[v]; seen {
				continue
			}
			hop[v] = hop[u] + 1
			queue = append(queue, v)
		}
	}

	const unreached = 1 << 30
	ordered := make([]string, 0, len(members))
	for id := range members {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		ha, haOK := hop[a]
		hb, hbOK := hop[b]
		if !haOK {
			ha = unreached
		}
		if !hbOK {
			hb = unreached
		}
		if ha != hb {
			return ha < hb
		}
		return a < b
	})
	return ordered
}

// reachableFrom is a multi-source BFS over edges, skipping any node in killed (and any
// edge touching one) entirely. killed == nil means no exclusions — the "before" state.
func reachableFrom(edges []DirectedEdge, entryPoints []string, killed map[string]bool) map[string]bool {
	adj := adjacency(edges)
	reachable := map[string]bool{}
	var queue []string

	for _, ep := range entryPoints {
		if killed[ep] {
			continue // an entry point that is itself killed reaches nothing
		}
		if !reachable[ep] {
			reachable[ep] = true
			queue = append(queue, ep)
		}
	}

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if reachable[v] || killed[v] {
				continue
			}
			reachable[v] = true
			queue = append(queue, v)
		}
	}
	return reachable
}
