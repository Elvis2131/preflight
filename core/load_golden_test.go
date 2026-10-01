package core_test

// PC-126/PC-149: proves ComputeComponentLoad against the real golden AWS bundle. The
// checkout journey now reaches its first component (internet -> ALB is Allowed under
// the assumed default NACL, PC-149) and blocks at the workload's security group, so
// the ALB carries the journey's declared load and nothing past it does.

import (
	"testing"

	"preflight/core"
)

func TestGoldenWorkload_ComponentLoad_HonestlyReflectsBlockedFlow(t *testing.T) {
	ir := realGoldenIR(t)
	workload := loadGoldenWorkload(t)

	loads := core.ComputeComponentLoad(ir, workload, nil)
	sawLB := false
	for _, l := range loads {
		switch l.NodeID {
		case "aws_lb.payments":
			sawLB = true
			if l.OfferedRPS != 800 {
				t.Errorf("aws_lb.payments OfferedRPS = %v, want 800 (checkout peak_rps)", l.OfferedRPS)
			}
		case "aws_eks_cluster.payments", "aws_db_instance.payments":
			t.Errorf("%s must not appear in the load report — checkout is blocked before reaching it, got %+v", l.NodeID, l)
		}
	}
	if !sawLB {
		t.Error("aws_lb.payments must appear in the load report — the first hop is Allowed")
	}
}
