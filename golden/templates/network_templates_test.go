package templates_test

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"preflight/core"
	"preflight/golden/templates"
	"preflight/ingest"
)

func networkTemplate(t *testing.T, id string) templates.Template {
	t.Helper()
	tpl, err := templates.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	return tpl
}

func networkNodes(tpl templates.Template) map[string]core.CanvasNode {
	byID := map[string]core.CanvasNode{}
	for _, n := range tpl.Canvas.Nodes {
		byID[n.ID] = n
	}
	return byID
}

func networkParents(tpl templates.Template, id string, edgeType core.EdgeType, service string) []core.CanvasNode {
	var out []core.CanvasNode
	byID := networkNodes(tpl)
	for _, e := range tpl.Canvas.Edges {
		if e.From == id && e.Type == edgeType && byID[e.To].ServiceID == service {
			out = append(out, byID[e.To])
		}
	}
	return out
}

func TestNetworkTemplates_AddressingAndNetworkControls(t *testing.T) {
	for _, id := range []string{"simple-aws-network", "enterprise-aws-network"} {
		t.Run(id, func(t *testing.T) {
			tpl := networkTemplate(t, id)
			if violations := core.ValidateCanvasNetworkControls(tpl.Canvas); len(violations) != 0 {
				t.Fatalf("invalid network controls: %+v", violations)
			}
			byID := networkNodes(tpl)
			vpcRanges := []netip.Prefix{}
			subnetRanges := map[string][]netip.Prefix{}
			for _, n := range tpl.Canvas.Nodes {
				if n.ServiceID != "aws_vpc" && n.ServiceID != "aws_subnet" {
					continue
				}
				prefix, err := netip.ParsePrefix(n.CIDRBlock)
				if err != nil {
					t.Fatalf("%s: invalid address range: %v", n.ID, err)
				}
				if n.ServiceID == "aws_vpc" {
					for _, previous := range vpcRanges {
						if previous.Overlaps(prefix) {
							t.Fatalf("overlapping VPC ranges: %s and %s", previous, prefix)
						}
					}
					vpcRanges = append(vpcRanges, prefix)
					continue
				}
				parents := networkParents(tpl, n.ID, core.EdgeTypeContainedIn, "aws_vpc")
				if len(parents) != 1 {
					t.Fatalf("%s needs exactly one VPC", n.ID)
				}
				parentRange, _ := netip.ParsePrefix(parents[0].CIDRBlock)
				if prefix.Bits() <= parentRange.Bits() || !parentRange.Contains(prefix.Addr()) {
					t.Fatalf("%s is outside its VPC", n.ID)
				}
				for _, previous := range subnetRanges[parents[0].ID] {
					if previous.Overlaps(prefix) {
						t.Fatalf("overlapping subnets in %s", parents[0].ID)
					}
				}
				subnetRanges[parents[0].ID] = append(subnetRanges[parents[0].ID], prefix)
				if len(networkParents(tpl, n.ID, core.EdgeTypeDependsOn, "aws_route_table")) != 1 {
					t.Errorf("%s must have one explicit route-table association", n.ID)
				}
				if len(networkParents(tpl, n.ID, core.EdgeTypeDependsOn, "aws_network_acl")) != 1 {
					t.Errorf("%s must have one explicit ACL association", n.ID)
				}
			}
			for _, e := range tpl.Canvas.Edges {
				if _, ok := byID[e.From]; !ok {
					t.Errorf("dangling source %s", e.From)
				}
				if _, ok := byID[e.To]; !ok {
					t.Errorf("dangling target %s", e.To)
				}
			}
			for _, n := range tpl.Canvas.Nodes {
				for _, rule := range n.SecurityGroupRules {
					if rule.SourceSecurityGroup != "" && byID[rule.SourceSecurityGroup].ServiceID != "aws_security_group" {
						t.Errorf("%s references an invalid source security group", n.ID)
					}
				}
				if n.ServiceID == "aws_subnet" && strings.Contains(n.ID, "_data_") {
					tables := networkParents(tpl, n.ID, core.EdgeTypeDependsOn, "aws_route_table")
					if len(tables) == 1 && len(tables[0].Routes) != 0 {
						t.Errorf("%s must be isolated with no internet/NAT default route", n.ID)
					}
				}
			}
		})
	}
}

func TestEnterpriseNetwork_NATEgressStaysInTheSameAZ(t *testing.T) {
	tpl := networkTemplate(t, "enterprise-aws-network")
	byID := networkNodes(tpl)
	for _, subnet := range tpl.Canvas.Nodes {
		if subnet.ServiceID != "aws_subnet" || !strings.Contains(subnet.ID, "_app_") {
			continue
		}
		tables := networkParents(tpl, subnet.ID, core.EdgeTypeDependsOn, "aws_route_table")
		if len(tables) != 1 || len(tables[0].Routes) != 1 {
			t.Fatalf("%s needs one NAT default route", subnet.ID)
		}
		route := tables[0].Routes[0]
		if route.DestinationCIDR != "0.0.0.0/0" || byID[route.Target].ServiceID != "aws_nat_gateway" {
			t.Fatalf("%s has the wrong default route", subnet.ID)
		}
		placements := networkParents(tpl, route.Target, core.EdgeTypeContainedIn, "aws_subnet")
		if len(placements) != 1 || placements[0].AvailabilityZone != subnet.AvailabilityZone {
			t.Errorf("%s depends on a NAT in another AZ", subnet.ID)
		}
		vpc := networkParents(tpl, subnet.ID, core.EdgeTypeContainedIn, "aws_vpc")[0].ID
		if networkParents(tpl, placements[0].ID, core.EdgeTypeContainedIn, "aws_vpc")[0].ID != vpc {
			t.Errorf("%s uses another VPC's NAT", subnet.ID)
		}
	}
}

func TestEnterpriseNetwork_TransitPolicySeparatesEnvironments(t *testing.T) {
	tpl := networkTemplate(t, "enterprise-aws-network")
	hub := networkNodes(tpl)["aws_ec2_transit_gateway.hub"]
	if hub.Capability["default_route_table_association"] != "disable" || hub.Capability["default_route_table_propagation"] != "disable" {
		t.Fatal("default TGW routing must not connect every environment")
	}
	var policy map[string]struct {
		Associated []string `json:"associated_attachments"`
		Routes     []struct {
			CIDR       string `json:"cidr"`
			Attachment string `json:"attachment"`
			Blackhole  bool   `json:"blackhole"`
		} `json:"routes"`
	}
	if err := json.Unmarshal([]byte(hub.Capability["design_route_tables"]), &policy); err != nil {
		t.Fatal(err)
	}
	for _, env := range []struct{ name, denied string }{{"production", "10.50.0.0/16"}, {"nonproduction", "10.40.0.0/16"}, {"hybrid", "10.50.0.0/16"}} {
		blocked := false
		for _, route := range policy[env.name].Routes {
			if route.CIDR == env.denied && route.Blackhole {
				blocked = true
			}
		}
		if !blocked {
			t.Errorf("%s does not explicitly block %s", env.name, env.denied)
		}
	}
	// This checks the saved policy intent, not an enforcement claim: the TGW gate
	// below must stay not_assessable until a routing model is implemented.
}

func TestNetworkTemplates_ModelledFlowsAndHonestLimits(t *testing.T) {
	reg := registry(t)
	for _, id := range []string{"simple-aws-network", "enterprise-aws-network"} {
		t.Run(id, func(t *testing.T) {
			tpl := networkTemplate(t, id)
			res, err := ingest.IngestCanvas(tpl.Canvas, reg, 1)
			if err != nil || res.IR == nil {
				t.Fatalf("ingest: %v", err)
			}
			flows := map[string]core.JourneyFlowResult{}
			for _, j := range tpl.Workload.Journeys {
				flows[j.ID] = core.ComputeJourneyFlow(res.IR, j, nil)
			}
			web := "simple_web"
			if id == "enterprise-aws-network" {
				web = "production_web"
			}
			if !flows[web].Flows {
				t.Fatalf("HTTPS entry is blocked: %s", flows[web].BlockedReason)
			}
			if id == "simple-aws-network" {
				if flows["simple_app"].Flows || !strings.Contains(flows["simple_app"].BlockedReason, "capability_check") {
					t.Error("unmapped EC2 must not falsely claim request simulation")
				}
			} else {
				if !flows["production_api"].Flows {
					t.Errorf("production TLS app hop blocked: %s", flows["production_api"].BlockedReason)
				}
				if flows["production_data"].Flows || !strings.Contains(flows["production_data"].BlockedReason, "route_selection") {
				t.Errorf("local-only data route must remain unknown until modelled, not be faked with a NAT route: %+v", flows["production_data"])
				}
				trace := core.BuildTrace(res.IR, "aws_eks_cluster.nonproduction", "aws_ec2_transit_gateway.hub", "", "tcp", 443)
				if trace.Allowed || trace.Steps[len(trace.Steps)-1].Decision != core.TraceNotAssessable {
					t.Error("TGW network intent must not become an assessed allow")
				}
				// The SG rules are load-bearing even in a partial architecture.
				for i := range tpl.Canvas.Nodes {
					if tpl.Canvas.Nodes[i].ID == "aws_security_group.prod_app" {
						tpl.Canvas.Nodes[i].SecurityGroupRules = tpl.Canvas.Nodes[i].SecurityGroupRules[1:]
					}
				}
				mutated, _ := ingest.IngestCanvas(tpl.Canvas, reg, 2)
				trace = core.BuildTrace(mutated.IR, "aws_lb.production", "aws_eks_cluster.production", "", "tcp", 8443)
				if trace.Allowed || !strings.Contains(trace.Concise, "sg_dest_ingress") {
					t.Error("removing the app's TLS ingress rule must block at the SG step")
				}
			}
		})
	}
}
