package core_test

// PC-113's own acceptance criterion, hand-verified directly: "Evaluated at subnet
// boundaries, so traffic between resources in the same subnet does not cross a NACL"
// — golden/aws has no NACL resources at all (confirmed: no aws_network_acl anywhere in
// this project as of PC-78/79's own scope decisions), so this is necessarily
// synthetic, hand-built coverage, the same discipline PC-14 established for
// core/internal/analyse before it ever touched real data.

import (
	"testing"

	"preflight/core"
)

func TestEvaluateNACLPath_SameSubnet_NeverEvaluatesTheNACL(t *testing.T) {
	// A profile that would DENY everything if it were ever actually evaluated — if
	// this test passed with evaluated=false BUT allowed=true, that would prove the
	// same-subnet short-circuit is real, not a coincidence of a permissive profile.
	denyAll := core.NACLProfile{NACLID: "acl-x", Rules: []core.NACLRule{
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: core.NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	evaluated, allowed, _, _ := core.EvaluateNACLPath("subnet.a", "subnet.a", denyAll, denyAll, "10.0.0.1/32", "10.0.0.2/32", "tcp", 443, 0, 0)
	if evaluated {
		t.Fatal("same-subnet traffic must never evaluate the NACL at all")
	}
	if !allowed {
		t.Fatal("same-subnet traffic must be treated as not blocked by a NACL — the NACL never applied at all, deny-all profile notwithstanding")
	}
}

func TestEvaluateNACLPath_DifferentSubnets_DoesEvaluate(t *testing.T) {
	denyAll := core.NACLProfile{NACLID: "acl-x", Rules: []core.NACLRule{
		{Number: core.NACLCatchAll, Direction: "ingress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
		{Number: core.NACLCatchAll, Direction: "egress", Protocol: "-1", CIDR: "0.0.0.0/0", Allow: false},
	}}
	evaluated, allowed, _, _ := core.EvaluateNACLPath("subnet.a", "subnet.b", denyAll, denyAll, "10.0.0.1/32", "10.0.0.2/32", "tcp", 443, 0, 0)
	if !evaluated {
		t.Fatal("cross-subnet traffic must evaluate the NACL")
	}
	if allowed {
		t.Fatal("a deny-all NACL on a real cross-subnet path must actually deny")
	}
}
