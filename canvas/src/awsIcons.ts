// awsIcons.ts (PC-109): the service → official AWS icon lookup. PRESENTATION ONLY — keyed by
// registry resource IDs and the design catalogue's explicit UI-only service identities;
// nothing about assessment, simulation or any verdict depends on it. The icons are AWS's own,
// from the AWS Architecture Icons package (Icon-package_07312026), copied byte for byte into
// public/aws-icons/ — see public/aws-icons/ICONS-LICENSE.md for the licence position, what
// was and was not verified, and the rules this file keeps (never altered, never implying AWS
// endorsement, and NO icon rather than a wrong one).

import { AWS_DIRECTORY_SERVICES } from "./awsServiceDirectory";
import { awsOnlyCatalog } from "./awsCatalog";
import { AWS_SERVICE_ICON_FILES, AWS_CATEGORY_ICON_FILES, AWS_CORE_CATEGORY_ICON_FILES } from "./awsServiceIconFiles";
import type { NodeType } from "./goldenVocabulary";

const directoryLabels = new Map(AWS_DIRECTORY_SERVICES.map((s) => [s.resource_type, s.display_name]));
const servicesByID = new Map(awsOnlyCatalog([]).map((s) => [s.resource_type, s]));

export const ICON_PACKAGE = {
  version: "07312026",
  source: "https://aws.amazon.com/architecture/icons/",
} as const;

// Resource variants may share the parent service's icon. Model support is
// resolved separately by the backend; this table controls presentation only.
export const SERVICE_ICON_FILES = AWS_SERVICE_ICON_FILES;

// The group icons AWS draws on VPC / subnet / Region boundaries.
export const GROUP_ICON_FILES = {
  vpc: "Virtual-private-cloud-VPC_32.svg",
  region: "Region_32.svg",
  publicSubnet: "Public-subnet_32.svg",
  privateSubnet: "Private-subnet_32.svg",
} as const;

// Friendly names for the core infrastructure resources and common abbreviations.
// Other names come from the AWS product directory.
export const SERVICE_LABELS: Readonly<Record<string, string>> = {
  aws_instance: "Amazon EC2",
  aws_autoscaling_group: "EC2 Auto Scaling",
  aws_ecs_service: "Amazon ECS",
  aws_ecr_repository: "Amazon ECR",
  aws_rds_cluster: "Amazon Aurora",
  aws_docdb_cluster: "Amazon DocumentDB",
  aws_keyspaces_table: "Amazon Keyspaces",
  aws_dsql_cluster: "Amazon Aurora DSQL",
  aws_kms_key: "AWS KMS",
  aws_acm_certificate: "AWS Certificate Manager",
  aws_sfn_state_machine: "AWS Step Functions",
  aws_cloudwatch_event_bus: "Amazon EventBridge",
  aws_msk_cluster: "Amazon MSK",
  aws_lambda_function: "AWS Lambda",
  aws_dynamodb_table: "Amazon DynamoDB",
  aws_eks_cluster: "Amazon EKS",
  aws_elasticache_replication_group: "Amazon ElastiCache",
  aws_db_instance: "Amazon RDS",
  aws_lb: "Elastic Load Balancing",
  aws_route53_record: "Amazon Route 53",
  aws_s3_bucket: "Amazon S3",
  aws_sqs_queue: "Amazon SQS",
  aws_sns_topic: "Amazon SNS",
  aws_wafv2_web_acl: "AWS WAF",
  aws_iam_role: "AWS IAM",
  aws_internet_gateway: "Internet gateway",
  aws_nat_gateway: "NAT gateway",
  aws_network_acl: "Network ACL",
  aws_default_network_acl: "Default network ACL",
  aws_route_table: "Route table",
  aws_security_group: "Security group",
  aws_subnet: "Subnet",
  aws_vpc: "VPC",
  aws_db_subnet_group: "DB subnet group",
  aws_elasticache_subnet_group: "ElastiCache subnet group",
};

function url(file: string): string {
  return `${import.meta.env.BASE_URL}aws-icons/${file}`;
}

// A dedicated service/resource icon takes priority over an explicitly labelled
// category icon. Unknown identities return null for the caller's generic mark.
export function iconDetailsForService(serviceID: string | undefined): { src: string; kind: "service" | "category" | "group"; title: string } | null {
  if (!serviceID) return null;
  const label = labelForService(serviceID);
  if (serviceID === "aws_vpc") return { src: groupIcon("vpc"), kind: "group", title: label };
  if (serviceID === "aws_subnet") return { src: groupIcon("privateSubnet"), kind: "group", title: label };
  const file = SERVICE_ICON_FILES[serviceID];
  if (file) return { src: url(file), kind: "service", title: label };
  const service = servicesByID.get(serviceID);
  if (!service) return null;
  const categoryFile = (service.category && AWS_CATEGORY_ICON_FILES[service.category]) || AWS_CORE_CATEGORY_ICON_FILES[service.node_type as NodeType];
  if (!categoryFile) return null;
  return {
    src: url(categoryFile),
    kind: "category",
    title: `${label} · AWS category icon (no dedicated icon in this release)`,
  };
}

export function iconForService(serviceID: string | undefined): string | null {
  return iconDetailsForService(serviceID)?.src ?? null;
}

export function groupIcon(kind: keyof typeof GROUP_ICON_FILES): string {
  return url(GROUP_ICON_FILES[kind]);
}

export function labelForService(serviceID: string): string {
  return SERVICE_LABELS[serviceID] ?? directoryLabels.get(serviceID) ?? serviceID;
}
