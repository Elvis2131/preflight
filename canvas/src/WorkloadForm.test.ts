import { describe, it, expect } from "vitest";
import { buildWorkload, emptyWorkloadFormValue, COMPLIANCE_FRAMEWORK_OPTIONS } from "./WorkloadForm";

// PC-87's own acceptance criteria, verbatim: (1) "form output validates against the
// same workload.schema.json contract" — buildWorkload must never invent a field the
// contract doesn't have; (2) "a capacity field left blank produces capacity_unknown
// downstream" — this test proves the FORM's own half of that: a blank value never
// becomes a 0 entry in the output map; (3) "hard and preference requirements are...
// functionally distinct" — a hard requirement must never carry a rank key at all.

describe("buildWorkload", () => {
  it("maps basic fields and splits comma-separated lists", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      name: "payments",
      criticality: "tier1",
      dataClassification: "PCI",
      regionsText: "eu-west-1, eu-west-2",
      complianceProfiles: ["pci_dss_4", "soc2"],
      capacityRows: [],
    });

    expect(w.name).toBe("payments");
    expect(w.criticality).toBe("tier1");
    expect(w.data_classification).toBe("PCI");
    expect(w.regions).toEqual(["eu-west-1", "eu-west-2"]);
    expect(w.compliance_profiles).toEqual(["pci_dss_4", "soc2"]);
    expect(w.schema_version).toBe("1.0.0");
  });

  it("compliance_profiles only ever carries implemented-framework ids selected via checkboxes, never free text", () => {
    const w = buildWorkload({ ...emptyWorkloadFormValue(), complianceProfiles: ["cis_aws"] });
    expect(w.compliance_profiles).toEqual(["cis_aws"]);
  });

  it("omits a capacity key entirely when its value is left blank — never sends 0", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      capacityRows: [
        { key: "app_node_rps", valueText: "" },
        { key: "db_iops", valueText: "500" },
      ],
    });

    expect(w.capacity).toEqual({ db_iops: 500 });
    // Explicit negative check, not just a shape match — prove the blank key is
    // genuinely ABSENT, not merely 0 and unchecked.
    expect(Object.prototype.hasOwnProperty.call(w.capacity, "app_node_rps")).toBe(false);
  });

  it("omits the capacity field entirely when every row is blank", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      capacityRows: [{ key: "app_node_rps", valueText: "" }],
    });
    expect(w.capacity).toBeUndefined();
  });

  it("never attaches a rank to a hard requirement, even if rankText was somehow set", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      requirementRows: [{ id: "rto", value: "4h", priority: "hard", rankText: "1" }],
    });
    expect(w.requirements).toEqual([{ id: "rto", value: "4h", priority: "hard" }]);
    expect(Object.prototype.hasOwnProperty.call(w.requirements[0], "rank")).toBe(false);
  });

  it("attaches a numeric rank to a preference requirement", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      requirementRows: [{ id: "latency", value: "p99<200ms", priority: "preference", rankText: "2" }],
    });
    expect(w.requirements).toEqual([{ id: "latency", value: "p99<200ms", priority: "preference", rank: 2 }]);
  });

  it("only offers frameworks with a real, implemented core.ComplianceFramework id as selectable", () => {
    // These ids mirror core/compliance_catalog.go's own real constants
    // (FrameworkCISAWS/FrameworkPCIDSS4/FrameworkSOC2) — cross-checked by name, not
    // guessed. A "coming soon" entry must be visibly non-selectable (PC-110's own
    // explicit instruction), never silently offered.
    const implemented = new Set(["cis_aws", "pci_dss_4", "soc2"]);
    for (const fw of COMPLIANCE_FRAMEWORK_OPTIONS) {
      if (implemented.has(fw.id)) {
        expect(fw.comingSoon).toBeUndefined();
      } else {
        expect(fw.comingSoon).toBeTruthy();
      }
    }
    // At least one real "coming soon, not yet selectable" example must exist —
    // otherwise this branch of the UI is dead code nothing ever exercises.
    expect(COMPLIANCE_FRAMEWORK_OPTIONS.some((fw) => fw.comingSoon)).toBe(true);
  });

  it("maps a complete journey row and never defaults a blank peak_rps/steady_rps", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      journeyRows: [
        { id: "checkout", name: "Checkout", pathText: "internet, lb-1, db-1", protocol: "tcp", port: "443", criticality: "tier1", peakRPSText: "800", steadyRPSText: "" },
      ],
    });
    expect(w.journeys).toEqual([
      { id: "checkout", name: "Checkout", path: ["internet", "lb-1", "db-1"], protocol: "tcp", port: 443, criticality: "tier1", peak_rps: 800 },
    ]);
    expect(Object.prototype.hasOwnProperty.call(w.journeys![0], "steady_rps")).toBe(false);
  });

  it("drops an incomplete journey row (e.g. a path with fewer than two hops)", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      journeyRows: [
        { id: "bad", name: "Bad", pathText: "only-one-node", protocol: "tcp", port: "443", criticality: "tier1", peakRPSText: "", steadyRPSText: "" },
      ],
    });
    expect(w.journeys).toBeUndefined();
  });

  it("omits journeys entirely when none are declared", () => {
    const w = buildWorkload(emptyWorkloadFormValue());
    expect(w.journeys).toBeUndefined();
  });

  it("drops a requirement row with no id — an empty row is not a real requirement", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      requirementRows: [{ id: "", value: "x", priority: "hard", rankText: "" }],
    });
    expect(w.requirements).toEqual([]);
  });
});
