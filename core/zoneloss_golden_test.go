package core_test

// This file is PC-78's third acceptance criterion: "PC-14's zone-loss test re-run
// against the updated golden IR now reports a real, non-vacuous result (specific nodes
// lost, not 'no path')." Uses core.ContainmentBlastRadius (the reverse-containment-
// reachability query PC-78 turned out to need — see that function's doc comment for
// why SimulateLoss, a forward-reachability-from-entry-points query, was the wrong tool
// for this specific question, discovered while first trying to apply it here).
//
// STRENGTHENED after initial delivery: the first version of these tests only checked
// "does the expected node appear in the lost set" — sufficient to prove the result is
// non-vacuous, but not sufficient to prove it's CORRECT. A bug that over-reported
// losses (e.g. treating the whole VPC as lost) would have passed those tests. Rewritten
// to assert the COMPLETE lost set via reflect.DeepEqual against a hand-worked expected
// answer, the same discipline core/internal/analyse/mincut_test.go already applied to
// synthetic graphs before this code ever touched real data — "reports something now"
// and "reports the right thing" are different claims, and the complete set is what
// actually proves the latter.
import (
	"reflect"
	"sort"
	"testing"

	"preflight/core"
	"preflight/ingest"
	awsprovider "preflight/providers/aws"
)

func containmentEdgesOf(t *testing.T, dir string, reg awsprovider.Registry) (*core.IR, []core.DirectedEdge) {
	t.Helper()
	result, err := ingest.Ingest(dir, reg, 1)
	if err != nil {
		t.Fatalf("Ingest(%s): %v", dir, err)
	}
	var edges []core.DirectedEdge
	for _, e := range result.IR.Edges {
		if e.Type == core.EdgeTypeContainedIn {
			edges = append(edges, core.DirectedEdge{From: e.From, To: e.To})
		}
	}
	if len(edges) == 0 {
		t.Fatalf("%s: expected real contained_in edges from the mapped substrate (PC-78) — got none", dir)
	}
	return result.IR, edges
}

// TestZoneKill_CleanBundle_PublicA_ExactBlastRadius hand-verifies the COMPLETE lost
// set for killing AZ eu-west-1a's public subnet against golden/aws (the clean
// bundle), worked out from the actual Terraform:
//   - aws_nat_gateway.nat_a is placed ONLY in public_a (network.tf) — lost.
//   - aws_lb.payments spans all three public subnets (alb.tf's `subnets` list) — one
//     of its three placements is lost; ContainmentBlastRadius reports the structural
//     fact per containment edge, not a redundancy verdict (see PC-78's own note on
//     this — a caller layering SurvivingCapacity/AZ-count on top decides whether "2 of
//     3 subnets remain" means the ALB is actually still functional).
//   - Nothing else: aws_nat_gateway.nat_b/nat_c (different subnets), every other
//     subnet, aws_vpc.payments itself (the container, not something contained BY
//     public_a), and every non-network node (RDS, cache, queues, IAM roles, WAF) have
//     no containment path to public_a at all and must be absent.
func TestZoneKill_CleanBundle_PublicA_ExactBlastRadius(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, edges := containmentEdgesOf(t, "../golden/aws", reg)

	lost := core.ContainmentBlastRadius(edges, "aws_subnet.public_a")
	sort.Strings(lost)

	want := []string{"aws_lb.payments", "aws_nat_gateway.nat_a"}
	if !reflect.DeepEqual(lost, want) {
		t.Fatalf("lost = %v, want exactly %v — golden/aws/network.tf places nat_a only in public_a, and alb.tf spans all three public subnets; nothing else should be reachable from public_a via contained_in", lost, want)
	}
}

// TestZoneKill_CleanBundle_PrivateA_ExactBlastRadius: private_a's only tenant is EKS
// (eks.tf's vpc_config.subnet_ids includes private_a/b/c). Hand-worked expected set:
// exactly [aws_eks_cluster.payments] — no NAT gateway is placed in a private subnet
// (NAT gateways live in the PUBLIC subnets, per network.tf), and no other node
// references private_a at all.
func TestZoneKill_CleanBundle_PrivateA_ExactBlastRadius(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, edges := containmentEdgesOf(t, "../golden/aws", reg)

	lost := core.ContainmentBlastRadius(edges, "aws_subnet.private_a")
	sort.Strings(lost)

	want := []string{"aws_eks_cluster.payments"}
	if !reflect.DeepEqual(lost, want) {
		t.Fatalf("lost = %v, want exactly %v — private_a's only tenant in golden/aws is the EKS cluster", lost, want)
	}
}

// TestZoneKill_CleanBundle_DataA_ExactBlastRadius: data_a exists in network.tf but
// NOTHING references it — RDS/ElastiCache go through their own subnet-group
// indirection (aws_db_subnet_group/aws_elasticache_subnet_group), which is itself
// unmapped (an explicitly named, separate gap — see docs from PC-13/PC-78's own
// scoping notes). Hand-worked expected answer: EMPTY. This is as important a case as
// the non-empty ones — it proves the function doesn't fabricate a loss where no real
// containment edge exists, and documents a real, current blind spot (RDS/cache AZ
// placement isn't visible to zone-kill yet) rather than letting a silently-empty
// result be mistaken for "verified safe."
func TestZoneKill_CleanBundle_DataA_ExactBlastRadius_KnownBlindSpot(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, edges := containmentEdgesOf(t, "../golden/aws", reg)

	lost := core.ContainmentBlastRadius(edges, "aws_subnet.data_a")
	if len(lost) != 0 {
		t.Fatalf("lost = %v, want empty — RDS/ElastiCache's own subnet placement is not visible to zone-kill yet (aws_db_subnet_group/aws_elasticache_subnet_group are unmapped); if this now returns something, that gap has been closed and this test's own comment is stale, not failing for the reason it says", lost)
	}
}

// TestZoneKill_BrokenBundle_PublicA_ExactBlastRadius: same AZ, broken bundle. Defect 1
// removes nat_b/nat_c entirely (network.tf), but nat_a is unaffected in ITS OWN
// subnet's placement, so the hand-worked answer for public_a specifically is
// IDENTICAL to the clean bundle — the difference defect 1 introduces shows up when
// killing public_b/public_c (which lose their gateways too in the clean bundle, but
// have none left to lose in the broken one), not when killing public_a itself. Worth
// asserting explicitly rather than assuming "broken bundle" always means "different
// answer for every AZ."
func TestZoneKill_BrokenBundle_PublicA_ExactBlastRadius(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, edges := containmentEdgesOf(t, "../golden/aws-broken", reg)

	lost := core.ContainmentBlastRadius(edges, "aws_subnet.public_a")
	sort.Strings(lost)

	want := []string{"aws_lb.payments", "aws_nat_gateway.nat_a"}
	if !reflect.DeepEqual(lost, want) {
		t.Fatalf("lost = %v, want exactly %v", lost, want)
	}
}

// TestZoneKill_BrokenBundle_PublicB_ShowsDefect1_NothingLeftToLose: the meaningful
// broken-bundle difference. golden/aws-broken/network.tf's defect 1 removes
// nat_b/nat_c — killing public_b in the BROKEN bundle loses only the ALB's placement
// there (no gateway left to lose), whereas the CLEAN bundle would additionally lose
// nat_b. This is the actual, hand-worked proof that defect 1's consequence is visible
// through zone-kill, not just through node-count diffing.
func TestZoneKill_BrokenBundle_PublicB_ShowsDefect1_NothingLeftToLose(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, brokenEdges := containmentEdgesOf(t, "../golden/aws-broken", reg)
	_, cleanEdges := containmentEdgesOf(t, "../golden/aws", reg)

	brokenLost := core.ContainmentBlastRadius(brokenEdges, "aws_subnet.public_b")
	cleanLost := core.ContainmentBlastRadius(cleanEdges, "aws_subnet.public_b")
	sort.Strings(brokenLost)
	sort.Strings(cleanLost)

	wantBroken := []string{"aws_lb.payments"} // no nat_b left to lose — defect 1
	wantClean := []string{"aws_lb.payments", "aws_nat_gateway.nat_b"}

	if !reflect.DeepEqual(brokenLost, wantBroken) {
		t.Fatalf("broken bundle: lost = %v, want exactly %v — defect 1 already removed nat_b, so killing public_b has nothing left to take", brokenLost, wantBroken)
	}
	if !reflect.DeepEqual(cleanLost, wantClean) {
		t.Fatalf("clean bundle: lost = %v, want exactly %v", cleanLost, wantClean)
	}
}

// TestZoneKill_BrokenBundle_PrivateA_ExactBlastRadius_SingleAZDefect: defect 5 places
// the ENTIRE EKS node group in private_a alone (golden/aws-broken/eks.tf), unlike the
// clean bundle where the node group (via the cluster's vpc_config) spans all three
// private subnets. Hand-worked expected answer: exactly [aws_eks_cluster.payments] —
// same single-element result as the clean bundle's private_a case, but for a
// materially different, worse reason (total loss here, one-of-three there) that this
// structural fact alone cannot distinguish — that distinction is exactly the
// redundancy/capacity judgment PC-78's own comments already flag as a separate,
// later concern for a caller layering PC-14's SurvivingCapacity on top.
func TestZoneKill_BrokenBundle_PrivateA_ExactBlastRadius_SingleAZDefect(t *testing.T) {
	reg, err := awsprovider.Load()
	if err != nil {
		t.Fatalf("providers/aws.Load(): %v", err)
	}
	_, edges := containmentEdgesOf(t, "../golden/aws-broken", reg)

	lost := core.ContainmentBlastRadius(edges, "aws_subnet.private_a")
	sort.Strings(lost)

	want := []string{"aws_eks_cluster.payments"}
	if !reflect.DeepEqual(lost, want) {
		t.Fatalf("lost = %v, want exactly %v — defect 5 places the entire node group in private_a alone", lost, want)
	}
}
