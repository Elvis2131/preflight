import { describe, it, expect } from "vitest";
import { resolveRegion, rowInRegion, REGION_PRICED_TYPES } from "./region";

// The SAME cases core/region_test.go asserts — the two resolvers must agree.
describe("resolveRegion (mirror of core.ResolveComponentRegion, PC-110)", () => {
  const cases: Array<[string, string | undefined, string[], string | null, string | null]> = [
    ["component's own region wins over a single workload region", "us-east-1", ["eu-west-1"], "us-east-1", "component"],
    ["component's own region wins when the workload declares several", "us-east-1", ["eu-west-1", "eu-central-1"], "us-east-1", "component"],
    ["no component region: exactly one workload region is used", undefined, ["eu-west-1"], "eu-west-1", "workload"],
    ["a blank component region is not a region", "", ["eu-west-1"], "eu-west-1", "workload"],
    ["several workload regions and no component region: unresolved", undefined, ["eu-west-1", "eu-central-1"], null, null],
    ["no workload region and no component region: unresolved", undefined, [], null, null],
  ];
  for (const [name, comp, wl, region, source] of cases) {
    it(name, () => {
      const r = resolveRegion(comp, wl);
      if (region === null) {
        expect(r.resolved).toBe(false);
      } else {
        expect(r).toEqual({ resolved: true, region, source });
      }
    });
  }

  it("never picks the first of several declared regions", () => {
    const r = resolveRegion(undefined, ["eu-west-1", "eu-central-1"]);
    expect(r.resolved).toBe(false);
    if (!r.resolved) expect(r.reason).toContain("never assumed");
  });
});

describe("rowInRegion / priced types", () => {
  it("excludes a row recorded for another region but keeps a region-less one", () => {
    expect(rowInRegion("eu-west-1", "eu-west-1")).toBe(true);
    expect(rowInRegion("us-east-1", "eu-west-1")).toBe(false);
    expect(rowInRegion("", "eu-west-1")).toBe(true);
  });
  it("covers exactly the types the cost engine prices", () => {
    expect([...REGION_PRICED_TYPES].sort()).toEqual(["cache", "load_balancer", "managed_database"]);
  });
});
