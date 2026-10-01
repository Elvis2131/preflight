import { describe, it, expect } from "vitest";
import { SERVICE_ICON_FILES, GROUP_ICON_FILES, SERVICE_LABELS, iconForService, groupIcon, labelForService, ICON_PACKAGE } from "./awsIcons";
import sums from "../public/aws-icons/SHA256SUMS?raw";

// Every committed icon, read as raw text so the test needs no Node fs APIs (same ?raw
// technique the other structural tests use; a Node-only API would break `npm run build`).
const files = import.meta.glob("../public/aws-icons/*.svg", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
const byName = Object.fromEntries(Object.entries(files).map(([p, text]) => [p.split("/").pop()!, text]));

async function sha256(text: string): Promise<string> {
  const buf = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return Array.from(new Uint8Array(buf)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

describe("AWS icons (PC-109)", () => {
  it("records the package version they came from", () => {
    expect(ICON_PACKAGE.version).toBe("07312026");
    expect(ICON_PACKAGE.source).toContain("aws.amazon.com/architecture/icons");
  });

  it("every mapped service and group icon exists on disk, and every committed icon is referenced", () => {
    const referenced = new Set([...Object.values(SERVICE_ICON_FILES), ...Object.values(GROUP_ICON_FILES)]);
    for (const f of referenced) expect(byName[f], `${f} is mapped but missing from public/aws-icons`).toBeTruthy();
    for (const f of Object.keys(byName)) expect(referenced.has(f), `${f} is committed but unused`).toBe(true);
  });

  it("each file is a real SVG", () => {
    for (const [name, text] of Object.entries(byName)) expect(text.trimStart().startsWith("<svg") || text.includes("<svg"), name).toBe(true);
  });

  it("every icon is BYTE-FOR-BYTE what AWS published (SHA256SUMS) — AWS's rule is never to alter the icons", async () => {
    const expected = Object.fromEntries(sums.trim().split("\n").map((l) => { const [h, n] = l.split(/\s+/); return [n, h]; }));
    expect(Object.keys(expected).sort()).toEqual(Object.keys(byName).sort());
    for (const [name, text] of Object.entries(byName)) expect(await sha256(text), `${name} differs from the published file`).toBe(expected[name]);
  });

  it("the digest check really detects alteration (negative control)", async () => {
    const name = Object.keys(byName)[0];
    const altered = byName[name].replace("<svg", "<svg data-tampered=\"1\"");
    const expected = sums.split("\n").find((l) => l.endsWith(name))!.split(/\s+/)[0];
    expect(await sha256(altered)).not.toBe(expected);
  });

  it("a service with no official icon yields null (the labelled generic shape), never another service's icon", () => {
    for (const none of ["aws_security_group", "aws_route_table", "aws_db_subnet_group", "aws_elasticache_subnet_group", "aws_subnet", "aws_vpc", "aws_totally_made_up", ""]) {
      expect(iconForService(none)).toBeNull();
    }
    expect(iconForService(undefined)).toBeNull();
  });

  it("no two services share one icon file (that would make an icon claim to be two services)", () => {
    const vals = Object.values(SERVICE_ICON_FILES);
    expect(new Set(vals).size).toBe(vals.length);
  });

  it("every iconned service has a friendly label, and unlabelled services fall back to their resource type", () => {
    for (const id of Object.keys(SERVICE_ICON_FILES)) expect(SERVICE_LABELS[id], id).toBeTruthy();
    expect(labelForService("aws_something_new")).toBe("aws_something_new");
    expect(iconForService("aws_lambda_function")).toMatch(/\/aws-icons\/Arch_AWS-Lambda_48\.svg$/);
    expect(groupIcon("region")).toMatch(/\/aws-icons\/Region_32\.svg$/);
  });
});
