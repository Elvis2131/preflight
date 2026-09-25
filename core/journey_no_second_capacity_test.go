package core_test

// PC-124's own acceptance criterion, verbatim: "No second capacity concept
// introduced; test/grep confirms journeys reuse existing declared-capacity fields."
// Checked structurally, the same reflection technique already used for Scorecard's
// own "no composite score" guarantee: DeclaredJourney itself must carry no field
// whose name contains "capacity" (case-insensitive) — capacity is looked up from
// Workload.Capacity only, via JourneyCapacityKey, never declared a second time on the
// journey itself.

import (
	"reflect"
	"strings"
	"testing"

	"preflight/core"
)

func TestDeclaredJourneyHasNoOwnCapacityField(t *testing.T) {
	typ := reflect.TypeOf(core.DeclaredJourney{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if strings.Contains(strings.ToLower(name), "capacity") {
			t.Errorf("DeclaredJourney has a field %q — capacity must only ever come from Workload.Capacity (JourneyCapacityKey), never a second field on the journey itself", name)
		}
	}
}
