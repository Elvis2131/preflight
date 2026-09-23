package analyse

import "sort"

// DirectedEdge is analyse's own minimal graph representation — deliberately not
// core.Edge itself, so the min-cut algorithm has no dependency on IR shape at all and
// can be hand-verified against small synthetic graphs with no core.IR machinery in the
// way (exactly the isolation the user asked for before this touches the real pipeline).
type DirectedEdge struct {
	From, To string
}

// adjacency returns, for every distinct vertex mentioned in edges, the sorted list of
// its outgoing neighbors. Sorted — not Go's native (randomized) map iteration order —
// because I1 requires determinism: the same graph must produce the same max-flow
// augmenting-path sequence, and therefore the same reported cut, on every run.
func adjacency(edges []DirectedEdge) map[string][]string {
	adj := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, e := range edges {
		if seen[e.From] == nil {
			seen[e.From] = map[string]bool{}
		}
		if !seen[e.From][e.To] {
			seen[e.From][e.To] = true
			adj[e.From] = append(adj[e.From], e.To)
		}
	}
	for v := range adj {
		sort.Strings(adj[v])
	}
	return adj
}

// vertexSet collects every distinct vertex mentioned by edges, sorted.
func vertexSet(edges []DirectedEdge) []string {
	seen := map[string]bool{}
	for _, e := range edges {
		seen[e.From] = true
		seen[e.To] = true
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
