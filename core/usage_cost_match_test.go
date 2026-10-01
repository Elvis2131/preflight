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
