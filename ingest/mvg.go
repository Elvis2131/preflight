package ingest

import "preflight/core"

// entryPointNodeTypes and statefulNodeTypes are PC-12's Conversation question, answered
// as an explicit list rather than a parse-time judgment call — derived from the golden
// vocabulary (CLAUDE.md §14) and cross-checked against golden/workload.yaml's own
// `entry_points: [dns]` declaration, not invented independently of it.
//
// An entry point is a node type through which external traffic enters the
// architecture; a stateful node is one that holds data whose loss is itself a failure
// mode. Both lists are deliberately conservative for v1 — see docs/IR_DESIGN_NOTE.md's
// WAF discussion for the analogous "this doesn't cleanly fit" problem: `load_balancer`
// is included as an entry point even though DNS is the literal external-facing node,
// because a fragment containing only an ALB and a database (no DNS yet) is still a
// perfectly reasonable mid-authoring state to assess, not one that should be rejected
// as insufficient.
var entryPointNodeTypes = map[core.NodeType]bool{
	core.NodeTypeDNS:          true,
	core.NodeTypeLoadBalancer: true,
}

var statefulNodeTypes = map[core.NodeType]bool{
	core.NodeTypeManagedDatabase: true,
	core.NodeTypeCache:           true,
	core.NodeTypeQueueStream:     true,
	core.NodeTypeObjectStore:     true,
}

// InsufficientModel is PC-12's first acceptance criterion's shape: "a fragment below
// the MVG threshold returns structured insufficient_model with missing[] and
// guidance[], never findings." This is ingest's own output type, not one of PC-7's
// five frozen contracts — a candidate for promotion to a sixth contract if a future
// ticket decides callers outside this process need to parse it too; not decided here.
type InsufficientModel struct {
	Status   string   `json:"status"` // always "insufficient_model"
	Missing  []string `json:"missing"`
	Guidance []string `json:"guidance"`
}

// CheckMVG applies PC-12's stated threshold (Conversation: "≥1 entry point + ≥1
// stateful node... a starting point per PRD §12, still needs tuning against real
// fragments"). Returns nil when the graph clears the bar; the PRD §12 source text
// itself was not available, so the exact threshold formula is taken verbatim from this
// ticket's own Conversation, not re-derived or guessed at.
func CheckMVG(nodes []core.Node) *InsufficientModel {
	hasEntryPoint := false
	hasStateful := false
	for _, n := range nodes {
		if entryPointNodeTypes[n.Type] {
			hasEntryPoint = true
		}
		if statefulNodeTypes[n.Type] {
			hasStateful = true
		}
	}
	if hasEntryPoint && hasStateful {
		return nil
	}

	m := &InsufficientModel{Status: "insufficient_model"}
	if !hasEntryPoint {
		m.Missing = append(m.Missing, "entry_point")
		m.Guidance = append(m.Guidance, "Add at least one entry point (dns or load_balancer) so a request path into this architecture exists to assess.")
	}
	if !hasStateful {
		m.Missing = append(m.Missing, "stateful_node")
		m.Guidance = append(m.Guidance, "Add at least one stateful node (managed_database, cache, queue/stream, or object_store) — a fragment with no state has nothing for a failure scenario to threaten yet.")
	}
	return m
}
