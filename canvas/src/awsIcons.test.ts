import { describe, it, expect } from "vitest";
import { SERVICE_ICON_FILES, GROUP_ICON_FILES, iconDetailsForService, iconForService, groupIcon, labelForService, ICON_PACKAGE } from "./awsIcons";
import { AWS_CATEGORY_ICON_FILES, AWS_CORE_CATEGORY_ICON_FILES } from "./awsServiceIconFiles";
import { awsOnlyCatalog } from "./awsCatalog";
import sums from "../public/aws-icons/SHA256SUMS?raw";
import sourcePaths from "../public/aws-icons/SOURCE-PATHS.json?raw";

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
    const referenced = new Set([...Object.values(SERVICE_ICON_FILES), ...Object.values(GROUP_ICON_FILES), ...Object.values(AWS_CATEGORY_ICON_FILES), ...Object.values(AWS_CORE_CATEGORY_ICON_FILES)]);
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

  it("VPC and subnet use boundary icons, while unknown services never borrow an icon", () => {
    expect(iconForService("aws_vpc")).toMatch(/\/aws-icons\/Virtual-private-cloud-VPC_32\.svg$/);
    expect(iconForService("aws_subnet")).toMatch(/\/aws-icons\/Private-subnet_32\.svg$/);
    for (const none of ["aws_wafv2_web_acl_association", "aws_totally_made_up", ""]) {
      expect(iconForService(none)).toBeNull();
    }
    expect(iconForService(undefined)).toBeNull();
  });

  it("resource variants may share their parent service's icon without mixing different AWS services", () => {
    expect(iconForService("aws_dynamodb_global_table")).toBe(iconForService("aws_dynamodb_table"));
    expect(iconForService("aws_elasticache_subnet_group")).toBe(iconForService("aws_elasticache_replication_group"));
    expect(iconForService("aws_ecs_task_definition")).not.toBe(iconForService("aws_ecs_service"));
    const distinct = ["aws_instance", "aws_ecs_service", "aws_eks_cluster", "aws_service:bedrock", "aws_lambda_function"].map(iconForService);
    expect(new Set(distinct).size).toBe(distinct.length);
  });

  it("every iconned service has a friendly label, and unlabelled services fall back to their resource type", () => {
    for (const id of Object.keys(SERVICE_ICON_FILES)) expect(labelForService(id), id).not.toBe(id);
    expect(labelForService("aws_something_new")).toBe("aws_something_new");
    expect(iconForService("aws_lambda_function")).toMatch(/\/aws-icons\/Arch_AWS-Lambda_48\.svg$/);
    expect(groupIcon("region")).toMatch(/\/aws-icons\/Region_32\.svg$/);
  });

  it("every library entry has an AWS icon and directory gaps explicitly use category icons", () => {
    for (const entry of awsOnlyCatalog([])) expect(iconForService(entry.resource_type), entry.resource_type).toBeTruthy();
    expect(iconDetailsForService("aws_instance")?.kind).toBe("service");
    expect(iconDetailsForService("aws_service:bedrock")?.src).toContain("Arch_Amazon-Bedrock_48.svg");
    const missingDedicated = iconDetailsForService("aws_service:pricing-calculator");
    expect(missingDedicated?.kind).toBe("category");
    expect(missingDedicated?.src).toContain("Arch-Category_Cloud-Financial-Management_48.svg");
    expect(missingDedicated?.title).toContain("no dedicated icon");
    expect(iconDetailsForService("aws_route_table")?.src).not.toContain("Route-53");
  });

  it("every copied asset records its exact path in the official AWS archive", () => {
    const provenance = JSON.parse(sourcePaths) as { package: string; files: Record<string, string> };
    expect(provenance.package).toBe("Icon-package_07312026");
    expect(Object.keys(provenance.files).sort()).toEqual(Object.keys(byName).sort());
    for (const [name, path] of Object.entries(provenance.files)) {
      expect(path).toMatch(/^(Architecture-Service-Icons|Architecture-Group-Icons|Resource-Icons|Category-Icons)_07312026\//);
      expect(path.endsWith(name)).toBe(true);
    }
  });
});
