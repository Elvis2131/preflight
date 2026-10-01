import type { ServiceCatalogEntry } from "./api";
import { AWS_DIRECTORY_SERVICES } from "./awsServiceDirectory";

export interface AWSServiceCatalogEntry extends ServiceCatalogEntry {
  display_name?: string;
  category?: string;
  documentation_url?: string;
}

// Catalogue-only product identities are UI metadata, never provider resource IDs.
export function isDirectoryService(serviceID: string): boolean {
  return serviceID.startsWith("aws_service:");
}

// Infrastructure entries supplement the AWS product directory. Model support is
// always supplied by the backend registry, including when the UI works offline.
export const DEFAULT_AWS_SERVICE_CATALOG: ServiceCatalogEntry[] = [
  { resource_type: "aws_lambda_function", node_type: "compute", capability_level: "UNMODELED" },
  { resource_type: "aws_eks_cluster", node_type: "container_workload", capability_level: "UNMODELED" },
  { resource_type: "aws_db_instance", node_type: "managed_database", capability_level: "UNMODELED" },
  { resource_type: "aws_dynamodb_table", node_type: "managed_database", capability_level: "UNMODELED" },
  { resource_type: "aws_elasticache_replication_group", node_type: "cache", capability_level: "UNMODELED" },
  { resource_type: "aws_lb", node_type: "load_balancer", capability_level: "UNMODELED" },
  { resource_type: "aws_sqs_queue", node_type: "queue/stream", capability_level: "UNMODELED" },
  { resource_type: "aws_sns_topic", node_type: "queue/stream", capability_level: "UNMODELED" },
  { resource_type: "aws_s3_bucket", node_type: "object_store", capability_level: "UNMODELED" },
  { resource_type: "aws_route53_record", node_type: "dns", capability_level: "UNMODELED" },
  { resource_type: "aws_vpc", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_subnet", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_internet_gateway", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_nat_gateway", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_route_table", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_network_acl", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_security_group", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_wafv2_web_acl", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_db_subnet_group", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_elasticache_subnet_group", node_type: "network_boundary", capability_level: "UNMODELED" },
  { resource_type: "aws_iam_role", node_type: "identity", capability_level: "UNMODELED" },
];

export function awsOnlyCatalog(entries: ServiceCatalogEntry[]): AWSServiceCatalogEntry[] {
  const byID = new Map<string, AWSServiceCatalogEntry>();
  for (const entry of AWS_DIRECTORY_SERVICES) {
    byID.set(entry.resource_type, { ...entry, capability_level: "UNMODELED" });
  }
  for (const entry of DEFAULT_AWS_SERVICE_CATALOG) {
    byID.set(entry.resource_type, { ...byID.get(entry.resource_type), ...entry, capability_level: "UNMODELED" });
  }
  for (const entry of entries) {
    if (entry.resource_type.startsWith("aws_")) {
      byID.set(entry.resource_type, { ...byID.get(entry.resource_type), ...entry });
    }
  }
  return Array.from(byID.values());
}
