// awsIcons.ts (PC-109): the service → official AWS icon lookup. PRESENTATION ONLY — keyed by
// the capability registry's service ID (the same resource_type providers.Registry uses);
// nothing about assessment, simulation or any verdict depends on it. The icons are AWS's own,
// from the AWS Architecture Icons package (Icon-package_07312026), copied byte for byte into
// public/aws-icons/ — see public/aws-icons/ICONS-LICENSE.md for the licence position, what
// was and was not verified, and the rules this file keeps (never altered, never implying AWS
// endorsement, and NO icon rather than a wrong one).

export const ICON_PACKAGE = {
  version: "07312026",
  source: "https://aws.amazon.com/architecture/icons/",
} as const;

// service_id → icon file. A service with no matching official icon is deliberately ABSENT
// (security group, route table, DB / ElastiCache subnet groups, WAF association): it is drawn
// as a labelled generic shape, never as an icon for a different service.
export const SERVICE_ICON_FILES: Readonly<Record<string, string>> = {
  aws_lambda_function: "Arch_AWS-Lambda_48.svg",
  aws_dynamodb_table: "Arch_Amazon-DynamoDB_48.svg",
  aws_eks_cluster: "Arch_Amazon-Elastic-Kubernetes-Service_48.svg",
  aws_elasticache_replication_group: "Arch_Amazon-ElastiCache_48.svg",
  aws_db_instance: "Arch_Amazon-RDS_48.svg",
  aws_lb: "Arch_Elastic-Load-Balancing_48.svg",
  aws_route53_record: "Arch_Amazon-Route-53_48.svg",
  aws_s3_bucket: "Arch_Amazon-Simple-Storage-Service_48.svg",
  aws_sqs_queue: "Arch_Amazon-Simple-Queue-Service_48.svg",
  aws_sns_topic: "Arch_Amazon-Simple-Notification-Service_48.svg",
  aws_wafv2_web_acl: "Arch_AWS-WAF_48.svg",
  aws_iam_role: "Arch_AWS-Identity-and-Access-Management_48.svg",
  aws_internet_gateway: "Res_Amazon-VPC_Internet-Gateway_48.svg",
  aws_nat_gateway: "Res_Amazon-VPC_NAT-Gateway_48.svg",
  aws_network_acl: "Res_Amazon-VPC_Network-Access-Control-List_48.svg",
};

// The group icons AWS draws on VPC / subnet / Region boundaries.
export const GROUP_ICON_FILES = {
  vpc: "Virtual-private-cloud-VPC_32.svg",
  region: "Region_32.svg",
  publicSubnet: "Public-subnet_32.svg",
  privateSubnet: "Private-subnet_32.svg",
} as const;

// Friendly palette names for services that have an icon (the registry has only resource
// types). A service without an entry is shown by its resource_type.
export const SERVICE_LABELS: Readonly<Record<string, string>> = {
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

// iconForService returns the icon URL for a service, or null when there is no official icon
// for it — the caller then draws the labelled generic shape.
export function iconForService(serviceID: string | undefined): string | null {
  if (!serviceID) return null;
  const f = SERVICE_ICON_FILES[serviceID];
  return f ? url(f) : null;
}

export function groupIcon(kind: keyof typeof GROUP_ICON_FILES): string {
  return url(GROUP_ICON_FILES[kind]);
}

export function labelForService(serviceID: string): string {
  return SERVICE_LABELS[serviceID] ?? serviceID;
}
