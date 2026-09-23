package core_test

// This file is PC-14's third acceptance criterion run against the real golden
// fixture, not a synthetic stand-in. UPDATED after PC-78: network/security-group
// substrate (VPC/subnet/NAT/SG) is now mapped and produces real contained_in edges —
// see core/zoneloss_golden_test.go for that criterion's own, now-meaningful result
// (killing a real AZ reports real losses, including golden/aws-broken's flagship
// single-NAT SPOF's actual node).
//
// What THIS file still, correctly, reports as "no path": the golden IR's only entry
// points (dns, load_balancer) and only stateful nodes (managed_database, cache,
// queue/stream) still have no depends_on-style edge connecting them — a DIFFERENT gap
// from the one PC-78 closed. Terraform doesn't encode "EKS calls RDS" as a resource
// reference; that connectivity lives in security group ALLOW rules (explicitly out of
// scope for PC-78's mapping — see providers/aws/security_group.yaml's own scope
// decision: SG-to-SG permission edges belong to the Network Engine's SG/NACL
// evaluation, not Layer 1 containment) and application configuration. So this remains
// the honest, correct answer for entry-to-critical-node SPOF specifically — not a
// claim that compute-to-data connectivity is unmodeled because nobody looked, but
// because modeling it correctly is a different, larger piece of work than containment
// was.
import (
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func TestDetectSPOFs_AgainstGoldenBundle(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	result, err := ingest.Ingest("../golden/aws", reg, 1)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	edges := make([]core.DirectedEdge, len(result.IR.Edges))
	for i, e := range result.IR.Edges {
		edges[i] = core.DirectedEdge{From: e.From, To: e.To}
	}

	entryPoints := nodeIDsOfType(result.IR.Nodes, core.NodeTypeDNS, core.NodeTypeLoadBalancer)
	criticalNodes := nodeIDsOfType(result.IR.Nodes, core.NodeTypeManagedDatabase, core.NodeTypeCache, core.NodeTypeQueueStream)

	if len(entryPoints) == 0 || len(criticalNodes) == 0 {
		t.Fatalf("expected at least one entry point and one critical node in the golden bundle; got entry=%v critical=%v", entryPoints, criticalNodes)
	}

	candidates := core.DetectSPOFs(edges, entryPoints, criticalNodes)
	if len(candidates) != len(entryPoints)*len(criticalNodes) {
		t.Fatalf("got %d candidates, want %d (every entry x critical pair)", len(candidates), len(entryPoints)*len(criticalNodes))
	}

	// The honest, current, hand-verified result: EVERY pair reports no path — see
	// this file's own doc comment for exactly why that's the correct answer given
	// today's provider mapping scope, not a bug in DetectSPOFs itself.
	for _, c := range candidates {
		if c.IsSPOF() {
			t.Errorf("%s -> %s: unexpectedly reported as a SPOF — the golden bundle's compute/entry tier has no modeled edge to its data tier today; if this starts passing, either connectivity mapping was added (update this test's expectation) or something is now producing a spurious edge", c.EntryPoint, c.CriticalNode)
		}
		if c.CutSize != 0 || c.Uncuttable {
			t.Errorf("%s -> %s: got CutSize=%d Uncuttable=%v, want CutSize=0 Uncuttable=false (no path modeled)", c.EntryPoint, c.CriticalNode, c.CutSize, c.Uncuttable)
		}
	}

	// What DOES have real connectivity in today's graph: the WAF association PC-12's
	// follow-up fix added. Confirmed here as a genuine, currently-detectable
	// structural fact, even though it isn't an entry-to-critical-node SPOF check.
	wafCandidates := core.DetectSPOFs(edges, []string{"aws_route53_record.api"}, []string{"aws_wafv2_web_acl.payments"})
	if len(wafCandidates) != 1 {
		t.Fatalf("expected exactly one candidate for dns -> waf")
	}
	// dns -> lb -> waf: lb sits between them, so it IS the min-cut vertex on this
	// specific path — a real, hand-verifiable result given the actual 2-hop chain.
	if !wafCandidates[0].IsSPOF() || wafCandidates[0].CutVertices[0] != "aws_lb.payments" {
		t.Errorf("got %+v, want a single-vertex cut at aws_lb.payments (dns -> lb -> waf is the only path)", wafCandidates[0])
	}
}

func nodeIDsOfType(nodes []core.Node, types ...core.NodeType) []string {
	want := map[core.NodeType]bool{}
	for _, t := range types {
		want[t] = true
	}
	var ids []string
	for _, n := range nodes {
		if want[n.Type] {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
