package core

import "testing"

// Real attribute shapes from the AmazonEC2 eu-west-1 offer (retrieved 2026-10-02):
// the zonal and regional NAT products carry the same unit and price but are
// different SKUs. The old substring match let the regional one satisfy the zonal one.
func TestUsageMatchers_RealShapes(t *testing.T) {
	zonal := PriceRow{Unit: "GB", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Bytes", "operation": "NatGateway"}}
	regional := PriceRow{Unit: "GB", SKUAttributes: map[string]string{"usagetype": "EU-RegionalNatGateway-Bytes", "operation": "RegionalNatGateway"}}
	usEast := PriceRow{Unit: "GB", SKUAttributes: map[string]string{"usagetype": "NatGateway-Bytes", "operation": "NatGateway"}}
	hours := PriceRow{Unit: "Hrs", SKUAttributes: map[string]string{"usagetype": "EU-NatGateway-Hours", "operation": "NatGateway"}}

	if !matchNATDataProcessingRow(zonal) || !matchNATDataProcessingRow(usEast) {
		t.Error("zonal NatGateway-Bytes (prefixed and unprefixed) must match")
	}
	if matchNATDataProcessingRow(regional) {
		t.Error("RegionalNatGateway-Bytes must not match the zonal dimension")
	}
	if matchNATDataProcessingRow(hours) {
		t.Error("NatGateway-Hours is an hourly charge, not data processing")
	}
	lcu := PriceRow{Unit: "LCU-Hrs", SKUAttributes: map[string]string{"usagetype": "LCUUsage", "operation": "LoadBalancing:Application"}}
	if !matchLBDataProcessedRow(lcu) {
		t.Error("ALB LCUUsage must match")
	}
	lcu.Unit = "GB"
	if matchLBDataProcessedRow(lcu) {
		t.Error("LCUUsage with an unexpected unit must not match")
	}
}

// Real eu-west-1 AWSELB rows (offer 20260911124544). The Outposts row has the same operation
// and unit as the on-demand row but is a $0 Outposts rate; picking it would price an ALB at zero.
func TestUsageMatchers_RealELBDecoys(t *testing.T) {
	onDemand := PriceRow{Unit: "LCU-Hrs", SKUAttributes: map[string]string{"usagetype": "EU-LCUUsage", "operation": "LoadBalancing:Application"}}
	outposts := PriceRow{Unit: "LCU-Hrs", SKUAttributes: map[string]string{"usagetype": "EU-Outposts-LCUUsage", "operation": "LoadBalancing:Application"}}
	reserved := PriceRow{Unit: "ReservedLCU-Hr", SKUAttributes: map[string]string{"usagetype": "EU-ReservedLCUUsage", "operation": "LoadBalancing:Application"}}
	if !matchLBDataProcessedRow(onDemand) {
		t.Error("the on-demand EU-LCUUsage row must match")
	}
	if matchLBDataProcessedRow(outposts) {
		t.Error("the $0 Outposts rate must never match")
	}
	if matchLBDataProcessedRow(reserved) {
		t.Error("the reserved-capacity rate must never match")
	}
	for u, want := range map[string]bool{"EU-NatGateway-Bytes": true, "NatGateway-Bytes": true, "USE1-NatGateway-Bytes": true, "EU-Outposts-NatGateway-Bytes": false} {
		got := usageTypeIs(PriceRow{SKUAttributes: map[string]string{"usagetype": u}}, "NatGateway-Bytes")
		if got != want {
			t.Errorf("usageTypeIs(%q, NatGateway-Bytes) = %v, want %v", u, got, want)
		}
	}
}
