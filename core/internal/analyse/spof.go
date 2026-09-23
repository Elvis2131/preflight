// This file is PC-14's third acceptance criterion: "SPOF (min-cut) output matches a
// hand-verified expected result on the golden fixture." It builds on mincut.go
// (already hand-verified in isolation against small synthetic graphs — see
// mincut_test.go — before ever touching real graph data, per the explicit sequencing
// this ticket started with).
//
// Like every other file in this package, no dependency on preflight/core: SPOFCandidate
// takes/returns plain strings, and the caller (core/spof.go) is responsible for
// resolving node IDs into real core.Node values and attaching provenance.
package analyse

// SPOFCandidate is one (entryPoint, criticalNode) pair's structural analysis result.
type SPOFCandidate struct {
	EntryPoint   string
	CriticalNode string

	// Uncuttable mirrors MinVertexCutResult.Uncuttable — a direct edge with zero
	// intermediate vertices, distinct from both "no path" and "a real N-vertex cut".
	Uncuttable bool

	// CutSize == 0 with Uncuttable == false means no dependency path exists between
	// this pair at all — not a SPOF (nothing to protect), and NOT the same thing as
	// "resilient" (redundant paths would report CutSize > 1) — see IsSPOF.
	CutSize     int
	CutVertices []string
}

// IsSPOF reports whether this candidate represents an actual single point of failure:
// exactly one vertex whose removal disconnects the entry point from the critical node.
// CutSize == 0 (no path exists) is explicitly NOT a SPOF — CLAUDE.md §14's own
// golden-scenario framing is about a dependency that exists and is fragile, not an
// absent dependency, and conflating the two would silently misreport "not connected at
// all" as "perfectly safe."
func (c SPOFCandidate) IsSPOF() bool {
	return !c.Uncuttable && c.CutSize == 1
}

// DetectSPOFs runs MinVertexCut for every (entryPoint, criticalNode) pair and returns
// one SPOFCandidate per pair — including the ones that are NOT single points of
// failure (CutSize == 0 or CutSize >= 2), so a caller can see the full picture (what
// was checked, not only what failed) rather than only ever seeing findings.
func DetectSPOFs(edges []DirectedEdge, entryPoints, criticalNodes []string) []SPOFCandidate {
	var candidates []SPOFCandidate
	for _, entry := range entryPoints {
		for _, critical := range criticalNodes {
			if entry == critical {
				continue // a node is never a SPOF for itself
			}
			result := MinVertexCut(edges, entry, critical)
			candidates = append(candidates, SPOFCandidate{
				EntryPoint:   entry,
				CriticalNode: critical,
				Uncuttable:   result.Uncuttable,
				CutSize:      result.Size,
				CutVertices:  result.CutVertices,
			})
		}
	}
	return candidates
}
