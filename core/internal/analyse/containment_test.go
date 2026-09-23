package analyse

import (
	"reflect"
	"testing"
)

// nat_a contained_in subnet_a contained_in vpc. Killing subnet_a should lose nat_a
// (contained within it) but not vpc (subnet_a is contained in vpc, not the reverse)
// and not an unrelated nat_b in a different subnet.
func TestContainmentBlastRadius_KillingSubnet_LosesWhatsInsideIt(t *testing.T) {
	edges := []DirectedEdge{
		{"nat_a", "subnet_a"}, {"subnet_a", "vpc"},
		{"nat_b", "subnet_b"}, {"subnet_b", "vpc"},
	}
	lost := ContainmentBlastRadius(edges, "subnet_a")
	if !reflect.DeepEqual(lost, []string{"nat_a"}) {
		t.Fatalf("lost = %v, want [nat_a] — nat_b is in a different subnet, vpc contains subnet_a rather than the reverse", lost)
	}
}

// Multiple levels: app contained_in subnet contained_in vpc. Killing the VPC loses
// everything transitively inside it.
func TestContainmentBlastRadius_MultipleLevels_TransitiveLoss(t *testing.T) {
	edges := []DirectedEdge{
		{"app", "subnet"}, {"subnet", "vpc"},
	}
	lost := ContainmentBlastRadius(edges, "vpc")
	if !reflect.DeepEqual(lost, []string{"app", "subnet"}) {
		t.Fatalf("lost = %v, want [app subnet]", lost)
	}
}

// A node spanning multiple containers (e.g. an ALB in 3 subnets) is lost only when
// killing ITS OWN container, not a sibling — but if it depends on ALL THREE for full
// capacity, that's a capacity/redundancy question, not this function's job. This test
// confirms ContainmentBlastRadius reports it as lost via EACH container independently
// (the caller decides what "still functional with 2 of 3" means, if anything).
func TestContainmentBlastRadius_MultiContainerNode_LostViaEachContainerIndependently(t *testing.T) {
	edges := []DirectedEdge{
		{"alb", "subnet_a"}, {"alb", "subnet_b"}, {"alb", "subnet_c"},
	}
	lost := ContainmentBlastRadius(edges, "subnet_a")
	if !reflect.DeepEqual(lost, []string{"alb"}) {
		t.Fatalf("lost = %v, want [alb] (subnet_a's own containment relationship)", lost)
	}
}

func TestContainmentBlastRadius_KilledNodeItself_NotInResult(t *testing.T) {
	edges := []DirectedEdge{{"child", "parent"}}
	lost := ContainmentBlastRadius(edges, "parent")
	for _, l := range lost {
		if l == "parent" {
			t.Fatal("the killed node itself must not appear in its own blast radius")
		}
	}
}

func TestContainmentBlastRadius_NoChildren_EmptyResult(t *testing.T) {
	edges := []DirectedEdge{{"a", "b"}}
	lost := ContainmentBlastRadius(edges, "a") // "a" has no children (nothing --> a)
	if len(lost) != 0 {
		t.Fatalf("lost = %v, want empty — nothing is contained in a leaf node", lost)
	}
}
