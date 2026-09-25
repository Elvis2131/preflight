// This file is PC-79's engine: security-group rule evaluation for compute-to-data SPOF
// detection, additive to (never replacing) PC-14's existing topology-based
// DetectSPOFs/MinVertexCut — see this file's own package doc comment below for the
// full scope statement, and core/spof_golden_test.go's doc comment for the exact gap
// this closes (dns/lb -> database/cache/queue currently reports "no path" because
// Terraform doesn't encode application-level connectivity as a resource reference;
// that connectivity lives in security-group ALLOW rules instead).
//
// CORRECTION, recorded rather than rewritten (this file's own established convention —
// see core/finding.go's own DetectionState comment for the precedent): PC-79 was
// closed against a stale ticket description. A comment already on PC-79 before this
// file was written had resolved the "SG-only vs SG+NACL" question below as "both, as
// separate engines" (PC-112 for Security Groups, PC-113 for NACLs) and said to leave
// PC-79 open until PC-112 closes; that comment was missed. PC-79 has been reopened.
// PC-112 owns the one real SG evaluation function (stateful, allow-only, CIDR + SG-
// reference sources, multi-SG union, protocol/port, explainable decision) — this
// file's own sgPermits is superseded scope (ingress-only, no CIDR, not stateful), not
// the final SG semantics, and will be re-implemented as a call into PC-112's evaluator.
// The tests below stay as a permanent regression check either way (PC-112's own
// reconciliation note says so explicitly). The scope reasoning immediately below
// remains accurate as a description of what THIS file does and why, at the time it was
// written — it is simply no longer the last word on where SG evaluation ends up.
//
// SCOPE DECISION, recorded per PC-79's first acceptance criterion: SG-only for v1, not
// SG+NACL. Reason: no aws_network_acl (or any NACL) resource exists anywhere in this
// project — not in golden/aws, golden/aws-broken, golden/azure, golden/azure-broken,
// nor in providers/aws's own mapping vocabulary. Building NACL evaluation now would be
// speculative generality against a resource type this codebase has never even ingested
// once (PRD §4's scenario-driven minimalism, the same rule PC-79's own Card cites
// against building this ticket ahead of schedule in the first place).
//
// FURTHER SCOPE DECISION: ingress-only, and SG-to-SG source-reference rules only — not
// CIDR-based rules. golden/aws/security.tf's own real shape is exactly this: the
// alb->workload->database/cache chain is gated entirely by source_security_group_id
// references (both the separate aws_security_group_rule resource form and an inline
// ingress block's security_groups list), and CIDR-based rules in that same file
// (0.0.0.0/0 on the ALB's public ingress, and every egress block) are about
// internet/outbound exposure, not compute-to-data reachability — the specific
// question this ticket's Card asks. A CIDR-based ingress rule would need to be
// evaluated against a NODE's actual IP, which this IR does not model at all (nodes are
// resource identities, not addresses) — so "does a CIDR rule permit this compute node"
// is not an honestly answerable question with today's IR, and is not attempted here.
//
// WHAT IS NOT BUILT HERE, deliberately, per PC-79's own acceptance criteria (a
// synthetic-fixture-only ticket, explicitly "before touching the golden
// architecture"): parsing real Terraform aws_security_group/aws_security_group_rule
// blocks into the SGRule/attachment maps below. That is ingest-side wiring work,
// scoped out of this ticket on purpose so the interface below could be settled first,
// per the Card's own instruction ("scope the interface before implementation so it
// doesn't quietly grow into a rewrite of PC-14"). FilterEdgesBySGRules takes those maps
// as plain input precisely so a later ticket can supply them from real ingest without
// this function changing at all.
package analyse

// SGRule is one security group's ingress ALLOW rule, restricted to the SG-to-SG
// source-reference shape this file evaluates (see the scope decision above). SourceSG
// is the security group ID this rule permits inbound traffic from — both
// golden/aws/security.tf's aws_security_group_rule.source_security_group_id and an
// inline ingress block's security_groups list resolve to this same shape.
type SGRule struct {
	SourceSG string
}

// FilterEdgesBySGRules keeps only the edges whose destination's attached security
// groups actually permit inbound traffic from the edge's source, per sgOf (resource ID
// -> attached security group IDs) and sgRules (security group ID -> its own ingress
// rules). PC-14's DetectSPOFs/MinVertexCut are called UNCHANGED on the result — this
// function only narrows the edge set they see; see this file's package doc comment for
// why that satisfies "no changes required to PC-14's existing topology-based SPOF
// logic" (PC-79's fourth acceptance criterion).
//
// An edge whose destination has no SGs recorded in sgOf at all is passed through
// unchanged, not blocked: most edges in a real IR (contained_in, and depends_on edges
// between resources with no SG modeled at all) have nothing to do with SG evaluation,
// and "no SG data exists for this edge" must never be silently reinterpreted as "this
// edge is blocked" — the same I4 discipline this codebase applies to Finding outcomes,
// carried one step earlier into a graph transform that feeds them. Only an edge whose
// destination DOES have at least one recorded SG, and where none of that SG's rules
// name one of the source's own attached SGs, is treated as blocked (dropped).
func FilterEdgesBySGRules(edges []DirectedEdge, sgOf map[string][]string, sgRules map[string][]SGRule) []DirectedEdge {
	out := make([]DirectedEdge, 0, len(edges))
	for _, e := range edges {
		if sgPermits(e.From, e.To, sgOf, sgRules) {
			out = append(out, e)
		}
	}
	return out
}

func sgPermits(from, to string, sgOf map[string][]string, sgRules map[string][]SGRule) bool {
	toSGs, ok := sgOf[to]
	if !ok || len(toSGs) == 0 {
		return true
	}
	fromSGs := sgOf[from]
	for _, sg := range toSGs {
		for _, rule := range sgRules[sg] {
			for _, fromSG := range fromSGs {
				if rule.SourceSG == fromSG {
					return true
				}
			}
		}
	}
	return false
}
