import { describe, expect, it } from "vitest";
import { awsOnlyCatalog } from "./awsCatalog";
import { NODE_TYPES } from "./goldenVocabulary";
import { labelForService } from "./awsIcons";

describe("AWS service catalog fallback", () => {
  it("contains the authored AWS canvas services even when the backend catalog is unavailable", () => {
    const ids = new Set(awsOnlyCatalog([]).map((e) => e.resource_type));
    for (const id of [
      "aws_vpc",
      "aws_subnet",
      "aws_route_table",
      "aws_security_group",
      "aws_network_acl",
      "aws_lambda_function",
      "aws_db_instance",
      "aws_lb",
    ]) {
      expect(ids.has(id), id).toBe(true);
    }
  });

  it("keeps the UI AWS-only and lets backend AWS rows override fallback rows", () => {
    const merged = awsOnlyCatalog([
      { resource_type: "google_sql_database", node_type: "managed_database", capability_level: "known" },
      { resource_type: "aws_lb", node_type: "load_balancer", capability_level: "backend" },
    ]);
    expect(merged.some((e) => !e.resource_type.startsWith("aws_"))).toBe(false);
    expect(merged.find((e) => e.resource_type === "aws_lb")?.capability_level).toBe("backend");
    expect(merged.length).toBe(awsOnlyCatalog([]).length);
    expect(merged.find((e) => e.resource_type === "aws_lb")?.documentation_url).toBeTruthy();
  });

  it("covers the AWS product directory with unique services assigned to real core types", () => {
    const catalog = awsOnlyCatalog([]);
    expect(catalog.length).toBeGreaterThan(250);
    expect(new Set(catalog.map((s) => s.resource_type)).size).toBe(catalog.length);
    for (const entry of catalog) {
      expect(NODE_TYPES).toContain(entry.node_type);
      expect(labelForService(entry.resource_type)).not.toBe(entry.resource_type);
      expect(labelForService(entry.resource_type)).not.toMatch(/azure/i);
    }
    for (const [id, group] of [
      ["aws_instance", "compute"], ["aws_ecs_service", "container_workload"],
      ["aws_rds_cluster", "managed_database"], ["aws_memorydb_cluster", "managed_database"],
      ["aws_cloudwatch_event_bus", "queue/stream"], ["aws_cloudfront_distribution", "load_balancer"],
      ["aws_route53_resolver_endpoint", "dns"], ["aws_networkfirewall_firewall", "network_boundary"],
      ["aws_cognito_user_pool", "identity"], ["aws_service:bedrock", "external_dependency"],
    ]) expect(catalog.find((s) => s.resource_type === id)?.node_type).toBe(group);
  });

  it("never treats directory breadth as backend simulation support", () => {
    expect(awsOnlyCatalog([]).every((s) => s.capability_level === "UNMODELED")).toBe(true);
    const merged = awsOnlyCatalog([{ resource_type: "aws_lambda_function", node_type: "compute", capability_level: "CONNECTIVITY" }]);
    expect(merged.find((s) => s.resource_type === "aws_lambda_function")?.capability_level).toBe("CONNECTIVITY");
    expect(merged.find((s) => s.resource_type === "aws_instance")?.capability_level).toBe("UNMODELED");
  });
});
