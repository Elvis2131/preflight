import { describe, it, expect } from "vitest";
import { buildWorkload, emptyWorkloadFormValue } from "./WorkloadForm";

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
      complianceProfilesText: "PCI, SOC2",
      capacityRows: [],
    });

    expect(w.name).toBe("payments");
    expect(w.criticality).toBe("tier1");
    expect(w.data_classification).toBe("PCI");
    expect(w.regions).toEqual(["eu-west-1", "eu-west-2"]);
    expect(w.compliance_profiles).toEqual(["PCI", "SOC2"]);
    expect(w.schema_version).toBe("1.0.0");
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

  it("drops a requirement row with no id — an empty row is not a real requirement", () => {
    const w = buildWorkload({
      ...emptyWorkloadFormValue(),
      requirementRows: [{ id: "", value: "x", priority: "hard", rankText: "" }],
    });
    expect(w.requirements).toEqual([]);
  });
});
