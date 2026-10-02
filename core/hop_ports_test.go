package core_test

// PC-152: a journey whose hops use different ports.

import (
	"strings"
	"testing"

	"preflight/core"
)

func TestHopPorts_DefaultAndOverride(t *testing.T) {
	j := core.DeclaredJourney{Port: 443, HopPorts: map[string]int{"app": 8080}}
	if j.PortForHop("app") != 8080 || j.PortForHop("db") != 443 {
		t.Errorf("override=%d default=%d, want 8080 and 443", j.PortForHop("app"), j.PortForHop("db"))
	}
	if (core.DeclaredJourney{Port: 22}).PortForHop("anything") != 22 {
		t.Error("a journey with no hop_ports behaves exactly as before")
	}
}

func TestHopPorts_Validate(t *testing.T) {
	base := core.DeclaredJourney{ID: "j", Name: "j", Path: []string{"internet", "lb", "app"}, Protocol: "tcp", Port: 443, Criticality: "tier1"}
	cases := []struct {
		name  string
		ports map[string]int
		path  []string
		want  string // substring of the error; "" = valid
	}{
		{"valid", map[string]int{"lb": 443, "app": 8080}, nil, ""},
		{"typo'd key falls back silently otherwise", map[string]int{"appp": 8080}, nil, "not the destination of any hop"},
		{"the journey's own start is not a hop destination", map[string]int{"internet": 80}, nil, "not the destination of any hop"},
		{"port out of range", map[string]int{"app": 70000}, nil, "HopPorts"},
		{"port zero", map[string]int{"app": 0}, nil, "HopPorts"},
		{"ambiguous repeated element", map[string]int{"lb": 80}, []string{"internet", "lb", "app", "lb"}, "more than once"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := base
			j.HopPorts = c.ports
			if c.path != nil {
				j.Path = c.path
			}
			err := core.Workload{SchemaVersion: "1.0.0", Name: "w", Criticality: "tier1", DataClassification: "internal", Regions: []string{"eu-west-1"}, Journeys: []core.DeclaredJourney{j}}.Validate()
			switch {
			case c.want == "" && err != nil:
				t.Errorf("valid hop_ports rejected: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Errorf("got %v, want an error containing %q", err, c.want)
			}
		})
	}
}

// The flow engine traces each hop with ITS port: with a single journey port the second
// hop is denied by the workload SG (which only admits 8080); with the real per-hop ports
// the whole path flows. Same IR, same journey path — only the declared ports differ.
func TestComputeJourneyFlow_UsesPerHopPorts_OnGolden(t *testing.T) {
	ir := realGoldenIR(t)
	path := []string{core.JourneyInternetSentinel, "aws_lb.payments", "aws_eks_cluster.payments", "aws_db_instance.payments"}
	single := core.DeclaredJourney{ID: "c", Name: "c", Path: path, Protocol: "tcp", Port: 443, Criticality: "tier1"}
	if f := core.ComputeJourneyFlow(ir, single, nil); f.Flows || f.BlockedAt != "aws_eks_cluster.payments" {
		t.Fatalf("one port (443) for every hop must still block at the workload SG: %+v", f)
	}
	perHop := single
	perHop.HopPorts = map[string]int{"aws_lb.payments": 443, "aws_eks_cluster.payments": 8080, "aws_db_instance.payments": 5432}
	f := core.ComputeJourneyFlow(ir, perHop, nil)
	if !f.Flows || len(f.Hops) != 3 {
		t.Fatalf("with each hop's real port the golden checkout path must flow end to end: %+v", f)
	}
	// And the override is load-bearing per hop: a wrong port on ONE hop blocks exactly there.
	wrong := perHop
	wrong.HopPorts = map[string]int{"aws_lb.payments": 443, "aws_eks_cluster.payments": 8080, "aws_db_instance.payments": 3306}
	if g := core.ComputeJourneyFlow(ir, wrong, nil); g.Flows || g.BlockedAt != "aws_db_instance.payments" {
		t.Errorf("a wrong port on the database hop must block at the database: %+v", g)
	}
}
