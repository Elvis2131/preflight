// region.ts (PC-110): the Design inspector's mirror of core.ResolveComponentRegion
// (core/region.go) — DISPLAY-SIDE GUIDANCE ONLY. It tells the architect which region the
// server will price a component in, and keeps the instance-type picker filtered to it; the
// price itself, and whether a component is priced at all, are the server's (cost_unknown
// naming "region" when none resolves). The same case table is asserted on both sides
// (core/region_test.go, region.test.ts), so the two cannot drift silently.
//
// The rule, from PC-110's decision comment — per component, with no guessed default:
//   1. the component's own region, if set;
//   2. otherwise the workload's region, but ONLY when it declares exactly one;
//   3. otherwise unresolved: the architect must choose, and until they do the component is
//      unpriced. The first of several declared regions is never picked.

export type RegionSource = "component" | "workload";

export type RegionResolution =
  | { resolved: true; region: string; source: RegionSource }
  | { resolved: false; reason: string };

export function resolveRegion(componentRegion: string | undefined, workloadRegions: string[]): RegionResolution {
  if (componentRegion && componentRegion.trim() !== "") {
    return { resolved: true, region: componentRegion.trim(), source: "component" };
  }
  if (workloadRegions.length === 1) {
    return { resolved: true, region: workloadRegions[0], source: "workload" };
  }
  if (workloadRegions.length === 0) {
    return { resolved: false, reason: "no region: the component declares none and the workload declares no region" };
  }
  return {
    resolved: false,
    reason: `no region: the workload declares ${workloadRegions.length} regions (${workloadRegions.join(", ")}) — choose which one this component runs in; the first is never assumed`,
  };
}

// REGION_PRICED_TYPES are the node types the cost engine prices today (core/cost.go), so
// the only ones for which "which region" changes a price.
export const REGION_PRICED_TYPES: ReadonlySet<string> = new Set(["managed_database", "cache", "load_balancer"]);

// rowInRegion mirrors core.priceRowInRegion: a row that records a region must match; a row
// with none recorded cannot be filtered, so it is not excluded.
export function rowInRegion(rowRegion: string, region: string): boolean {
  return rowRegion === "" || rowRegion === region;
}
