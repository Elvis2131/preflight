package core

import "fmt"

// PC-110: which region a component is priced in. Resolved PER COMPONENT, in this order,
// with no guessed default (PC-110's decision comment):
//
//  1. the component's own region (Sizing.Region, PC-115), if set;
//  2. otherwise the workload's declared region — but ONLY when the workload declares
//     exactly one (that list is stated input, so using it when it is unambiguous invents
//     nothing);
//  3. otherwise no region resolves: the component is unpriced (cost_unknown naming
//     "region" as the missing input) until the architect chooses one. Picking the first
//     of several declared regions would be a silent default, which the cost rules forbid.

// RegionSource records which step supplied a resolved region.
type RegionSource string

const (
	RegionFromComponent RegionSource = "component"
	RegionFromWorkload  RegionSource = "workload"
)

// ResolveComponentRegion applies the three-step rule. region is "" exactly when it did
// not resolve, in which case reason says why and what the architect must do.
func ResolveComponentRegion(sizing *Sizing, workloadRegions []string) (region string, source RegionSource, reason string) {
	if sizing != nil && sizing.Region != nil && *sizing.Region != "" {
		return *sizing.Region, RegionFromComponent, ""
	}
	switch len(workloadRegions) {
	case 1:
		return workloadRegions[0], RegionFromWorkload, ""
	case 0:
		return "", "", "region is missing: the component declares none and the workload declares no region — choose a region for this component"
	default:
		return "", "", fmt.Sprintf("region is missing: the component declares none and the workload declares %d regions (%v) — choose which region this component runs in (the first is never assumed)", len(workloadRegions), workloadRegions)
	}
}

// priceRowInRegion reports whether a snapshot row may price a component resolved to
// region. A row that carries a region must match it; a row with no region recorded (a
// snapshot that predates region scoping, e.g. a hand-built test fixture) cannot be
// filtered, so it is not excluded — its region claim is the snapshot's, not ours.
func priceRowInRegion(row PriceRow, region string) bool {
	return row.Region == "" || row.Region == region
}
