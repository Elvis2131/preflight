// This file is PC-78's actual missing piece, found while trying to run PC-14's
// zone-loss engine against the newly-mapped substrate: SimulateLoss answers "what
// becomes unreachable from entry points" (a dependency-path question), but AZ-kill
// blast radius is the OPPOSITE question — "what is transitively CONTAINED IN the
// killed node" (a containment-hierarchy question). Since PRD §4's contained_in edges
// point child -> parent (a NAT gateway's edge points TO its subnet, not the reverse),
// answering "what's inside subnet X" requires walking those edges backwards, which
// SimulateLoss's entry-point-forward-reachability model cannot express — a different
// graph query needs a different function, not a workaround bolted onto the existing
// one.
//
// Same package rule as everywhere else here: no dependency on preflight/core.
package analyse

import "sort"

// ContainmentBlastRadius returns every node with a path to killedNode via
// containmentEdges (assumed to be contained_in edges specifically — mixing in
// depends_on edges would answer a different, wrong question; filtering to the correct
// edge type is the caller's job, same division of labor as SimulateLoss's caller
// choosing which edges represent reachability). killedNode itself is never included —
// it was directly killed, which is a different fact from "lost as a consequence" (the
// same distinction SimulateLoss's own doc comment makes).
func ContainmentBlastRadius(containmentEdges []DirectedEdge, killedNode string) []string {
	// Reverse adjacency: parent -> its direct children (child --contained_in--> parent
	// means parent's reverse entry gains child).
	reverseAdj := map[string][]string{}
	for _, e := range containmentEdges {
		reverseAdj[e.To] = append(reverseAdj[e.To], e.From)
	}
	for k := range reverseAdj {
		sort.Strings(reverseAdj[k]) // determinism (I1) — same as adjacency() elsewhere
	}

	visited := map[string]bool{killedNode: true}
	queue := []string{killedNode}
	var lost []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, child := range reverseAdj[u] {
			if visited[child] {
				continue
			}
			visited[child] = true
			lost = append(lost, child)
			queue = append(queue, child)
		}
	}
	sort.Strings(lost)
	return lost
}
