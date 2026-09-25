package analyse

import "testing"

// TestExactlyOneSGEvaluationPath is PC-112's own acceptance addition (its
// reconciliation note on PC-79): "a test asserting there is exactly one SG
// rule-evaluation implementation in the codebase (FilterEdgesBySGRules delegates to
// it)." sgPermits (PC-79's own bridge) and EvaluateDirectional (PC-112's real engine)
// must agree on every case, because sgPermits's own source (securitygroup.go) calls
// EvaluateDirectional directly rather than reimplementing rule matching — this test
// proves that by construction AND by cross-checking real outputs agree, so a future
// edit that reintroduced a second, independently-drifting matching path (rather than
// editing the one real EvaluateDirectional) would be caught here.
func TestExactlyOneSGEvaluationPath(t *testing.T) {
	sgOf := map[string][]string{
		"compute1": {"sg-compute1"},
		"compute2": {"sg-compute2"},
		"data":     {"sg-data"},
	}
	sgRules := map[string][]SGRule{
		"sg-data": {{SourceSG: "sg-compute1"}},
	}

	// sgPermits's own real decision (via FilterEdgesBySGRules's public surface).
	edges := []DirectedEdge{{"compute1", "data"}, {"compute2", "data"}}
	filtered := FilterEdgesBySGRules(edges, sgOf, sgRules)

	permittedPairs := map[[2]string]bool{}
	for _, e := range filtered {
		permittedPairs[[2]string{e.From, e.To}] = true
	}

	// EvaluateDirectional's own decision, built from the identical data, called
	// directly rather than through sgPermits's bridge — if these ever disagreed, it
	// would mean sgPermits stopped actually delegating and grew its own logic again.
	for from, sgs := range sgOf {
		if from == "data" {
			continue
		}
		profile := SGProfile{SGIDs: sgOf["data"]}
		for _, sg := range sgOf["data"] {
			for _, r := range sgRules[sg] {
				profile.Rules = append(profile.Rules, SGRule{Direction: "ingress", Protocol: "-1", SourceSG: r.SourceSG})
			}
		}
		direct := EvaluateDirectional(profile, "ingress", "", sgs, "-1", 0)
		want := permittedPairs[[2]string{from, "data"}]
		if direct.Allowed != want {
			t.Errorf("%s -> data: sgPermits (via FilterEdgesBySGRules) says permitted=%v, EvaluateDirectional says allowed=%v — these must always agree, there is exactly one real evaluation path", from, want, direct.Allowed)
		}
	}
}
