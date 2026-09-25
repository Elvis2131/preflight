package core_test

// PC-126: proves ComputeComponentLoad against the real golden AWS bundle — honestly
// near-empty, since PC-125's own golden test already established that the real
// "checkout" journey is blocked on its very first hop (golden/aws has zero NACL
// resources, PC-113). This is the true, correct result, not a wished-for one.

import (
	"testing"

	"preflight/core"
)

func TestGoldenWorkload_ComponentLoad_HonestlyReflectsBlockedFlow(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)

	loads := core.ComputeComponentLoad(ir, workload, nil)
	for _, l := range loads {
		if l.NodeID == "aws_lb.payments" || l.NodeID == "aws_eks_cluster.payments" || l.NodeID == "aws_db_instance.payments" {
			t.Errorf("%s must not appear in the load report — the checkout journey never reaches past its first (blocked) hop, got %+v", l.NodeID, l)
		}
	}
}
