package ingest

// Hand-verified against small, constructed inputs before this is trusted against real
// golden-bundle data (core/failover_golden_test.go covers that end-to-end) — same
// discipline as every other engine in this codebase. PC-22: deriveFailover replaces
// core/internal/analyse's old DeriveNodeReplicationAndFailover — see ingest/build.go's
// own doc comment for why the relocation happened.

import (
	"testing"

	awsprovider "preflight/providers/aws"
)

func testFailoverMapping() awsprovider.FailoverMapping {
	return awsprovider.FailoverMapping{
		SourceAttribute: "multi_az",
		WhenEnabled: awsprovider.FailoverOutcome{
			ReplicationMode:   "sync",
			FailoverMechanism: "Multi-AZ automatic failover to a synchronous standby replica",
			Citation:          "test fixture, not a real doc citation",
		},
	}
}

func TestDeriveFailover_Enabled_ReturnsMappingDataVerbatim(t *testing.T) {
	r := ParsedResource{Attributes: map[string]any{"multi_az": true}}
	rm, fm := deriveFailover(r, testFailoverMapping())
	if rm == nil || *rm != "sync" {
		t.Fatalf("ReplicationMode = %v, want sync", rm)
	}
	if fm == nil || *fm != "Multi-AZ automatic failover to a synchronous standby replica" {
		t.Fatalf("FailoverMechanism = %v, want the mapping's exact when_enabled text", fm)
	}
}

func TestDeriveFailover_Disabled_ReturnsSharedSentinelNotYAMLData(t *testing.T) {
	r := ParsedResource{Attributes: map[string]any{"multi_az": false}}
	rm, fm := deriveFailover(r, testFailoverMapping())
	if rm == nil || *rm != "none" {
		t.Fatalf("ReplicationMode = %v, want the hardcoded \"none\" sentinel, not anything from YAML — the disabled case must never be freely-typed data (PC-14's sentinel-drift bug)", rm)
	}
	if fm == nil || *fm != "none" {
		t.Fatalf("FailoverMechanism = %v, want the hardcoded \"none\" sentinel", fm)
	}
}

func TestDeriveFailover_AttributeAbsent_IsNotAssessable(t *testing.T) {
	r := ParsedResource{Attributes: map[string]any{}}
	rm, fm := deriveFailover(r, testFailoverMapping())
	if rm != nil || fm != nil {
		t.Fatalf("got (%v, %v), want (nil, nil) — absence must never be guessed, no assumed-default path exists here", rm, fm)
	}
}

func TestDeriveFailover_AttributeWrongType_IsNotAssessable(t *testing.T) {
	r := ParsedResource{Attributes: map[string]any{"multi_az": "yes"}} // not a bool
	rm, fm := deriveFailover(r, testFailoverMapping())
	if rm != nil || fm != nil {
		t.Fatalf("got (%v, %v), want (nil, nil) for a non-bool source attribute value", rm, fm)
	}
}
