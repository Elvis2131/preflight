package core_test

// PC-126/PC-152: proves ComputeComponentLoad against the real golden AWS bundle. The checkout
// journey now flows through all three components, each offered its declared peak (800 rps)
// against golden's declared capacity: ALB 2000 (0.4), workload 500 (1.6), database 300
// (2.667). The workload and database are genuinely under-capacity for the declared peak:
// that is what golden/workload.yaml says, reported as such, not softened.

import (
	"math"
	"testing"

	"preflight/core"
)

func TestGoldenWorkload_ComponentLoad_HandVerified(t *testing.T) {
	loads := core.ComputeComponentLoad(realGoldenIR(t), loadGoldenWorkload(t), nil)
	want := map[string]struct{ capacity, utilization float64 }{
		"aws_lb.payments":          {2000, 0.4},
		"aws_eks_cluster.payments": {500, 1.6},
		"aws_db_instance.payments": {300, 800.0 / 300.0},
	}
	seen := map[string]bool{}
	for _, l := range loads {
		w, ok := want[l.NodeID]
		if !ok {
			continue
		}
		seen[l.NodeID] = true
		if l.OfferedRPS != 800 || l.Capacity == nil || *l.Capacity != w.capacity || l.Utilization == nil || math.Abs(*l.Utilization-w.utilization) > 1e-9 {
			t.Errorf("%s = %+v, want offered 800 against capacity %v (utilization %v)", l.NodeID, l, w.capacity, w.utilization)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s missing from the load report", id)
		}
	}
}
