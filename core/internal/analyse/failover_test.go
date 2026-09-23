package analyse

import "testing"

// Tests the plain-tuple signatures directly (this package has no core dependency);
// the core.Assessment[bool]-wrapped end-to-end behavior against the real golden
// bundles is tested in core/failover_golden_test.go.
//
// PC-22: TestRDSFailoverCapability, TestElastiCacheFailoverCapability, and
// TestDeriveNodeReplicationAndFailover used to live here, testing functions that have
// moved to providers/aws's own FailoverMapping data + ingest/build.go's deriveFailover
// (see failover.go's package doc comment for why). Their hand-verified coverage moved
// with them — see ingest/build_internal_test.go's TestDeriveFailover_* — rather than
// being dropped.

func TestRPOFeasibility(t *testing.T) {
	zero, hundred := 0.0, 100.0
	sync, async, unknown := "sync", "async", "unknown_mode"

	cases := []struct {
		name      string
		rpo       *float64
		mode      *string
		wantOK    bool
		wantValue bool
	}{
		{"sync satisfies RPO=0", &zero, &sync, true, true},
		{"sync satisfies any nonzero RPO too", &hundred, &sync, true, true},
		{"async fails RPO=0 definitionally", &zero, &async, true, false},
		{"async + nonzero RPO needs Rung-3 evidence, not-assessable", &hundred, &async, false, false},
		{"no declared RPO at all, not-assessable", nil, &sync, false, false},
		{"unknown replication mode, not-assessable", &zero, nil, false, false},
		{"unrecognized mode value, not-assessable", &zero, &unknown, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value, ok, reason := RPOFeasibility(c.rpo, c.mode)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (value=%v reason=%q)", ok, c.wantOK, value, reason)
			}
			if ok && value != c.wantValue {
				t.Errorf("value = %v, want %v", value, c.wantValue)
			}
			if !ok && reason == "" {
				t.Error("not-assessable result must carry a reason")
			}
		})
	}
}

func TestRTOFeasibility_NoMechanism_IsAssessedFalse(t *testing.T) {
	none := FailoverMechanismNone
	value, ok, _ := RTOFeasibility(&none)
	if !ok || value {
		t.Fatalf("got (value=%v ok=%v), want (false, true) — no automatic failover mechanism exists", value, ok)
	}
}

func TestRTOFeasibility_RealMechanism_IsAssessedTrue(t *testing.T) {
	mechanism := "Multi-AZ automatic failover to a synchronous standby replica"
	value, ok, _ := RTOFeasibility(&mechanism)
	if !ok || !value {
		t.Fatalf("got (value=%v ok=%v), want (true, true)", value, ok)
	}
}

func TestRTOFeasibility_UnknownMechanism_NotAssessable(t *testing.T) {
	_, ok, reason := RTOFeasibility(nil)
	if ok {
		t.Fatal("expected not-assessable when failover mechanism is unknown")
	}
	if reason == "" {
		t.Error("expected a non-empty reason")
	}
}

func TestRequirementValue(t *testing.T) {
	reqs := []Requirement{
		{ID: "rto_seconds", Value: 60},
		{ID: "availability.target", Value: 99.95},
	}
	if v, ok := RequirementValue(reqs, "rto_seconds"); !ok || v != 60 {
		t.Errorf("rto_seconds: got (%v, %v), want (60, true)", v, ok)
	}
	if v, ok := RequirementValue(reqs, "availability.target"); !ok || v != 99.95 {
		t.Errorf("availability.target: got (%v, %v), want (99.95, true)", v, ok)
	}
	if _, ok := RequirementValue(reqs, "nonexistent"); ok {
		t.Error("expected ok=false for a requirement that isn't declared")
	}
}
