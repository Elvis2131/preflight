// This file is PC-130's own "useful derived output," verbatim from the Card: "for
// each journey, the set of single rule changes that would break it (its
// 'configuration blast surface')." Scoped exactly as instructed: "compute over the
// rules actually on the journey's path, not every possible rule in the universe,"
// and capped, reporting the cap rather than silently truncating.
//
// Reuses ComputeJourneyFlow (PC-125) and WithSGRuleRemoved/WithNACLRuleRemoved
// (this ticket, above) verbatim for every candidate — no evaluation logic is
// reimplemented here, only "try removing this one real rule, see if the journey
// still flows."
package core

import "fmt"

// ConfigBlastSurfaceCap bounds how many candidate rules a single
// ComputeConfigurationBlastSurface call will actually test removing — the Card's own
// explicit instruction ("if the search space gets large, cap it and report the cap
// rather than silently truncating"). A journey's own path is normally a handful of
// hops with a handful of rules each; 50 is generous headroom over any real golden
// journey while still bounding a pathological case (many attached SGs, many rules
// each) to a fixed amount of work.
const ConfigBlastSurfaceCap = 50

// ConfigBlastSurfaceEntry names one real rule whose removal, alone, would break the
// journey — exactly one of SGNodeID/SGRule or NACLNodeID/NACLRule is set.
type ConfigBlastSurfaceEntry struct {
	SGNodeID   string
	SGRule     *SGRule
	NACLNodeID string
	NACLRule   *NACLRule
}

// ConfigBlastSurfaceResult is one journey's full configuration blast surface.
type ConfigBlastSurfaceResult struct {
	JourneyID       string
	BreakingChanges []ConfigBlastSurfaceEntry
	RulesConsidered int
	Capped          bool
}

// configRuleCandidate is one real rule found on the journey's own path, before it is
// tested — carries enough to both mutate the IR and de-duplicate (the same SG/NACL is
// often shared by more than one hop; each real rule is tested at most once).
type configRuleCandidate struct {
	sgNodeID   string
	sgRule     *SGRule
	naclNodeID string
	naclRule   *NACLRule
}

// key must identify the RULE'S FULL CONTENT, not just its direction/protocol — two
// distinct rules on the same SG can share a direction and protocol (e.g. two ingress
// tcp rules on different ports), and collapsing them onto the same key would silently
// test only one of them, never reporting the other as a candidate at all. Using
// fmt.Sprintf("%+v", ...) over the full struct (ports, CIDRs, source SG / CIDR /
// allow / number all included) avoids hand-maintaining a field list that could drift
// out of sync with SGRule/NACLRule's own fields.
func (c configRuleCandidate) key() string {
	if c.sgRule != nil {
		return "sg:" + c.sgNodeID + ":" + fmt.Sprintf("%+v", *c.sgRule)
	}
	return "nacl:" + c.naclNodeID + ":" + fmt.Sprintf("%+v", *c.naclRule)
}

// ComputeConfigurationBlastSurface finds every real SG/NACL rule actually consulted
// along j's declared Path (never the whole IR's rule universe) and reports which of
// them, removed alone, would break the journey — j is evaluated against ir exactly
// as ComputeJourneyFlow already would (killed may be nil, same as that function's own
// parameter), so this reflects whatever fault state the caller has already applied.
func ComputeConfigurationBlastSurface(ir *IR, j DeclaredJourney, killed map[string]bool) ConfigBlastSurfaceResult {
	result := ConfigBlastSurfaceResult{JourneyID: j.ID}

	baseline := ComputeJourneyFlow(ir, j, killed)
	if !baseline.Flows {
		// Nothing to compute a blast surface FOR — the journey is already broken by
		// something this fault-change search doesn't concern itself with (a killed
		// node, an unrelated block). Reported as a real, empty result, not an error.
		return result
	}

	seen := map[string]bool{}
	var candidates []configRuleCandidate

	for i := 0; i+1 < len(j.Path); i++ {
		from, to := j.Path[i], j.Path[i+1]

		for _, side := range []string{from, to} {
			if side == JourneyInternetSentinel || side == "" {
				continue
			}
			for _, sgID := range SecurityGroupProfile(ir.Nodes, ir.Edges, side).SGIDs {
				node, ok := findNode(ir, sgID)
				if !ok {
					continue
				}
				raw := rawRuleMaps(node.RawAttributes["security_group_rules"])
				for _, r := range raw {
					rule := toSGRule(r)
					c := configRuleCandidate{sgNodeID: sgID, sgRule: &rule}
					if !seen[c.key()] {
						seen[c.key()] = true
						candidates = append(candidates, c)
					}
				}
			}
			if subnetID, ok := resolveSubnetID(ir, side); ok {
				if profile, ok := NACLProfileForSubnet(ir.Nodes, ir.Edges, subnetID); ok {
					for _, r := range profile.Rules {
						rule := r
						c := configRuleCandidate{naclNodeID: profile.NACLID, naclRule: &rule}
						if !seen[c.key()] {
							seen[c.key()] = true
							candidates = append(candidates, c)
						}
					}
				}
			}
		}
	}

	result.RulesConsidered = len(candidates)
	if len(candidates) > ConfigBlastSurfaceCap {
		candidates = candidates[:ConfigBlastSurfaceCap]
		result.Capped = true
	}

	for _, c := range candidates {
		var mutated *IR
		var ok bool
		if c.sgRule != nil {
			mutated, ok = WithSGRuleRemoved(ir, c.sgNodeID, *c.sgRule)
		} else {
			mutated, ok = WithNACLRuleRemoved(ir, c.naclNodeID, *c.naclRule)
		}
		if !ok {
			continue // structurally shouldn't happen (the rule was just read off this same ir), but never guess
		}
		if flow := ComputeJourneyFlow(mutated, j, killed); !flow.Flows {
			result.BreakingChanges = append(result.BreakingChanges, ConfigBlastSurfaceEntry{
				SGNodeID: c.sgNodeID, SGRule: c.sgRule, NACLNodeID: c.naclNodeID, NACLRule: c.naclRule,
			})
		}
	}

	return result
}
