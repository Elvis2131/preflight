package core

import (
	"fmt"
	"sort"
	"strings"
)

// RenderDOT (PC-81) converts an IR into Graphviz DOT source — deterministic, pure,
// no I/O: the same *IR always produces byte-identical DOT text, which is what makes
// the eventual SVG determinism guarantee (NFR-1) possible at all. This is a STRING,
// not a rendered image — invoking the actual `dot` binary is real subprocess I/O and
// deliberately lives outside core/ (see render.SVG, the render/ package) per I1: core/
// must do no I/O of any kind.
//
// Determinism, stated explicitly since Go gives none of this for free: Nodes/Edges are
// sorted by ID before iteration (an *IR's own slice order is not trusted as stable,
// since nothing in this package's own contract requires a caller to hand them in
// sorted order) — this is the same "sort before any traversal that feeds output
// ordering" discipline CLAUDE.md's own NFR-1 names for exactly this reason (Go maps
// have deliberately randomized iteration order; this function uses none, but sorts
// anyway so it never depends on a caller's own incidental ordering either).
//
// Stable node IDs across versions (this ticket's own second acceptance criterion):
// each DOT node's identifier is the real Node.ID itself (a Terraform resource address,
// or a canvas-authored ID) — never an arbitrary counter — so the same real-world
// resource renders under the identical DOT/SVG node ID in every version that contains
// it, which is what makes a version-to-version diff comparable at all (PC-91's own
// eventual job, not reimplemented here).
func RenderDOT(ir *IR) string {
	nodes := make([]Node, len(ir.Nodes))
	copy(nodes, ir.Nodes)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edges := make([]Edge, len(ir.Edges))
	copy(edges, ir.Edges)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].ID < edges[j].ID
	})

	var b strings.Builder
	b.WriteString("digraph preflight {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, fontname=\"Helvetica\", fontsize=10];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", fontsize=9];\n")

	for _, n := range nodes {
		label := dotEscape(n.ID) + "\\n" + dotEscape(string(n.Type))
		style := ""
		if n.Resolution == ResolutionUnresolved {
			style = ", style=dashed, color=\"#b91c1c\""
		}
		fmt.Fprintf(&b, "  %s [label=%s%s];\n", dotQuoteID(n.ID), dotQuote(label), style)
	}

	for _, e := range edges {
		style := ""
		if e.Resolution == ResolutionUnresolved {
			style = ", style=dashed, color=\"#b91c1c\""
		}
		fmt.Fprintf(&b, "  %s -> %s [label=%s%s];\n",
			dotQuoteID(e.From), dotQuoteID(e.To), dotQuote(dotEscape(string(e.Type))), style)
	}

	b.WriteString("}\n")
	return b.String()
}

// dotQuoteID renders a node identifier as a DOT-safe quoted ID literal — node IDs are
// real-world strings (Terraform addresses like "aws_db_instance.payments", or canvas
// IDs like "managed_database-3") that can contain characters DOT's own unquoted
// identifier grammar does not allow (., -, /), so every node ID is always quoted,
// never emitted bare.
func dotQuoteID(id string) string { return dotQuote(dotEscape(id)) }

// dotEscape escapes the two characters DOT's quoted-string grammar treats specially
// (backslash, double-quote) — verified against Graphviz's own documented string
// literal escaping (only \" and \\ need escaping inside a quoted ID), not guessed.
func dotEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func dotQuote(s string) string { return `"` + s + `"` }
