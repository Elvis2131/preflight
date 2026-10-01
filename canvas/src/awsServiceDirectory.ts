// Snapshot of AWS's product documentation index, retrieved 2026-10-02.
// Source: https://docs.aws.amazon.com/
// Toolkits and guides are excluded. Group assignments describe the role in a
// design; they do not grant simulation or pricing support. aws_service: IDs are
// UI catalogue identities, not Terraform resources, and are not serialized.
import type { NodeType } from "./goldenVocabulary";

export interface AWSDirectoryService {
  resource_type: string;
  node_type: NodeType;
  display_name: string;
  category: string;
  documentation_url: string;
}

export const AWS_DIRECTORY_SERVICES: AWSDirectoryService[] = [
  {
    "resource_type": "aws_service:sagemaker",
    "node_type": "external_dependency",
    "display_name": "Amazon SageMaker",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/next-generation-sagemaker/"
  },
  {
    "resource_type": "aws_service:appflow",
    "node_type": "external_dependency",
    "display_name": "Amazon AppFlow",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/appflow/"
  },
  {
    "resource_type": "aws_service:athena",
    "node_type": "external_dependency",
    "display_name": "Amazon Athena",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/athena/"
  },
  {
    "resource_type": "aws_service:clean-rooms",
    "node_type": "external_dependency",
    "display_name": "AWS Clean Rooms",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/clean-rooms/"
  },
  {
    "resource_type": "aws_cloudsearch_domain",
    "node_type": "managed_database",
    "display_name": "Amazon CloudSearch",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/cloudsearch/"
  },
  {
    "resource_type": "aws_service:data-exchange",
    "node_type": "external_dependency",
    "display_name": "AWS Data Exchange",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/data-exchange/"
  },
  {
    "resource_type": "aws_kinesis_firehose_delivery_stream",
    "node_type": "queue/stream",
    "display_name": "Amazon Data Firehose",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/firehose/"
  },
  {
    "resource_type": "aws_service:data-pipeline",
    "node_type": "external_dependency",
    "display_name": "AWS Data Pipeline",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/data-pipeline/"
  },
  {
    "resource_type": "aws_service:datazone",
    "node_type": "external_dependency",
    "display_name": "Amazon DataZone",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/datazone/"
  },
  {
    "resource_type": "aws_emr_cluster",
    "node_type": "compute",
    "display_name": "Amazon EMR",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/emr/"
  },
  {
    "resource_type": "aws_service:entity-resolution",
    "node_type": "external_dependency",
    "display_name": "AWS Entity Resolution",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/entityresolution/"
  },
  {
    "resource_type": "aws_service:finspace",
    "node_type": "external_dependency",
    "display_name": "Amazon FinSpace",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/finspace/"
  },
  {
    "resource_type": "aws_glue_job",
    "node_type": "compute",
    "display_name": "AWS Glue",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/glue/"
  },
  {
    "resource_type": "aws_kinesis_stream",
    "node_type": "queue/stream",
    "display_name": "Amazon Kinesis",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/kinesis/"
  },
  {
    "resource_type": "aws_service:lake-formation",
    "node_type": "external_dependency",
    "display_name": "AWS Lake Formation",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/lake-formation/"
  },
  {
    "resource_type": "aws_kinesisanalyticsv2_application",
    "node_type": "compute",
    "display_name": "Amazon Managed Service for Apache Flink",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/managed-flink/"
  },
  {
    "resource_type": "aws_msk_cluster",
    "node_type": "queue/stream",
    "display_name": "Amazon Managed Streaming for Apache Kafka",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/msk/"
  },
  {
    "resource_type": "aws_opensearch_domain",
    "node_type": "managed_database",
    "display_name": "Amazon OpenSearch Service",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/opensearch-service/"
  },
  {
    "resource_type": "aws_service:quick",
    "node_type": "external_dependency",
    "display_name": "Amazon Quick",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/quick/"
  },
  {
    "resource_type": "aws_redshift_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon Redshift",
    "category": "Analytics",
    "documentation_url": "https://docs.aws.amazon.com/redshift/"
  },
  {
    "resource_type": "aws_service:b2b-data-interchange",
    "node_type": "external_dependency",
    "display_name": "AWS B2B Data Interchange",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/b2bi/"
  },
  {
    "resource_type": "aws_cloudwatch_event_bus",
    "node_type": "queue/stream",
    "display_name": "Amazon EventBridge",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/eventbridge/"
  },
  {
    "resource_type": "aws_mq_broker",
    "node_type": "queue/stream",
    "display_name": "Amazon MQ",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/amazon-mq/"
  },
  {
    "resource_type": "aws_mwaa_environment",
    "node_type": "external_dependency",
    "display_name": "Amazon MWAA",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/mwaa/"
  },
  {
    "resource_type": "aws_sns_topic",
    "node_type": "queue/stream",
    "display_name": "Amazon Simple Notification Service",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/sns/"
  },
  {
    "resource_type": "aws_sqs_queue",
    "node_type": "queue/stream",
    "display_name": "Amazon Simple Queue Service",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/sqs/"
  },
  {
    "resource_type": "aws_service:simple-workflow-service",
    "node_type": "external_dependency",
    "display_name": "Amazon Simple Workflow Service",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/swf/"
  },
  {
    "resource_type": "aws_sfn_state_machine",
    "node_type": "external_dependency",
    "display_name": "AWS Step Functions",
    "category": "Application Integration",
    "documentation_url": "https://docs.aws.amazon.com/step-functions/"
  },
  {
    "resource_type": "aws_service:managed-blockchain",
    "node_type": "external_dependency",
    "display_name": "Amazon Managed Blockchain",
    "category": "Blockchain",
    "documentation_url": "https://docs.aws.amazon.com/managed-blockchain/"
  },
  {
    "resource_type": "aws_service:connect-customer",
    "node_type": "external_dependency",
    "display_name": "Amazon Connect Customer",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/connect/"
  },
  {
    "resource_type": "aws_service:app-studio",
    "node_type": "external_dependency",
    "display_name": "AWS App Studio",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/appstudio/"
  },
  {
    "resource_type": "aws_service:appfabric",
    "node_type": "external_dependency",
    "display_name": "AWS AppFabric",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/appfabric/"
  },
  {
    "resource_type": "aws_service:chime",
    "node_type": "external_dependency",
    "display_name": "Amazon Chime",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/chime/"
  },
  {
    "resource_type": "aws_service:chime-sdk",
    "node_type": "external_dependency",
    "display_name": "Amazon Chime SDK",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/chime-sdk/"
  },
  {
    "resource_type": "aws_service:connect-decisions",
    "node_type": "external_dependency",
    "display_name": "Amazon Connect Decisions",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/connect-decisions/"
  },
  {
    "resource_type": "aws_service:connect-health",
    "node_type": "external_dependency",
    "display_name": "Amazon Connect Health",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/connecthealth/"
  },
  {
    "resource_type": "aws_service:connect-talent",
    "node_type": "external_dependency",
    "display_name": "Amazon Connect Talent",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/talent/"
  },
  {
    "resource_type": "aws_service:end-user-messaging-push",
    "node_type": "external_dependency",
    "display_name": "AWS End User Messaging Push",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/end-user-messaging/"
  },
  {
    "resource_type": "aws_service:end-user-messaging-sms",
    "node_type": "external_dependency",
    "display_name": "AWS End User Messaging SMS",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/sms-voice/"
  },
  {
    "resource_type": "aws_service:end-user-messaging-social",
    "node_type": "external_dependency",
    "display_name": "AWS End User Messaging Social",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/end-user-messaging/"
  },
  {
    "resource_type": "aws_service:pinpoint",
    "node_type": "external_dependency",
    "display_name": "Amazon Pinpoint",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/pinpoint/"
  },
  {
    "resource_type": "aws_service:simple-email-service",
    "node_type": "external_dependency",
    "display_name": "Amazon Simple Email Service",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/ses/"
  },
  {
    "resource_type": "aws_service:supply-chain",
    "node_type": "external_dependency",
    "display_name": "AWS Supply Chain",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/connect-decisions/"
  },
  {
    "resource_type": "aws_service:wickr",
    "node_type": "external_dependency",
    "display_name": "AWS Wickr",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/wickr/"
  },
  {
    "resource_type": "aws_service:workmail",
    "node_type": "external_dependency",
    "display_name": "Amazon WorkMail",
    "category": "Business Applications",
    "documentation_url": "https://docs.aws.amazon.com/workmail/"
  },
  {
    "resource_type": "aws_service:billing-and-cost-management",
    "node_type": "external_dependency",
    "display_name": "AWS Billing and Cost Management",
    "category": "Cloud Financial Management",
    "documentation_url": "https://docs.aws.amazon.com/account-billing/"
  },
  {
    "resource_type": "aws_service:pricing-calculator",
    "node_type": "external_dependency",
    "display_name": "AWS Pricing Calculator",
    "category": "Cloud Financial Management",
    "documentation_url": "https://docs.aws.amazon.com/pricing-calculator/"
  },
  {
    "resource_type": "aws_apprunner_service",
    "node_type": "container_workload",
    "display_name": "AWS App Runner",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/apprunner/"
  },
  {
    "resource_type": "aws_lightsail_instance",
    "node_type": "compute",
    "display_name": "Amazon Lightsail",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/lightsail/"
  },
  {
    "resource_type": "aws_batch_job_definition",
    "node_type": "compute",
    "display_name": "AWS Batch",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/batch/"
  },
  {
    "resource_type": "aws_service:ec2-image-builder",
    "node_type": "external_dependency",
    "display_name": "EC2 Image Builder",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/imagebuilder/"
  },
  {
    "resource_type": "aws_elastic_beanstalk_environment",
    "node_type": "compute",
    "display_name": "AWS Elastic Beanstalk",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/elastic-beanstalk/"
  },
  {
    "resource_type": "aws_instance",
    "node_type": "compute",
    "display_name": "Amazon Elastic Compute Cloud",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/ec2/"
  },
  {
    "resource_type": "aws_lambda_function",
    "node_type": "compute",
    "display_name": "AWS Lambda",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/lambda/"
  },
  {
    "resource_type": "aws_service:local-zones",
    "node_type": "external_dependency",
    "display_name": "AWS Local Zones",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/local-zones/"
  },
  {
    "resource_type": "aws_service:outposts",
    "node_type": "external_dependency",
    "display_name": "AWS Outposts",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/outposts/"
  },
  {
    "resource_type": "aws_pcs_cluster",
    "node_type": "compute",
    "display_name": "AWS Parallel Computing Service",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/pcs/"
  },
  {
    "resource_type": "aws_service:serverless-application-repository",
    "node_type": "external_dependency",
    "display_name": "AWS Serverless Application Repository",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/serverlessrepo/"
  },
  {
    "resource_type": "aws_service:wavelength",
    "node_type": "external_dependency",
    "display_name": "AWS Wavelength",
    "category": "Compute",
    "documentation_url": "https://docs.aws.amazon.com/wavelength/"
  },
  {
    "resource_type": "aws_service:research-and-engineering-studio-on-aws",
    "node_type": "external_dependency",
    "display_name": "Research and Engineering Studio on AWS",
    "category": "Compute HPC",
    "documentation_url": "https://docs.aws.amazon.com/res/"
  },
  {
    "resource_type": "aws_ecr_repository",
    "node_type": "external_dependency",
    "display_name": "Amazon Elastic Container Registry",
    "category": "Containers",
    "documentation_url": "https://docs.aws.amazon.com/ecr/"
  },
  {
    "resource_type": "aws_ecs_service",
    "node_type": "container_workload",
    "display_name": "Amazon Elastic Container Service",
    "category": "Containers",
    "documentation_url": "https://docs.aws.amazon.com/ecs/"
  },
  {
    "resource_type": "aws_eks_cluster",
    "node_type": "container_workload",
    "display_name": "Amazon Elastic Kubernetes Service",
    "category": "Containers",
    "documentation_url": "https://docs.aws.amazon.com/eks/"
  },
  {
    "resource_type": "aws_service:rosa",
    "node_type": "container_workload",
    "display_name": "Red Hat OpenShift Service on AWS",
    "category": "Containers",
    "documentation_url": "https://docs.aws.amazon.com/rosa/"
  },
  {
    "resource_type": "aws_acm_certificate",
    "node_type": "identity",
    "display_name": "AWS Certificate Manager",
    "category": "Cryptography & PKI",
    "documentation_url": "https://docs.aws.amazon.com/acm/"
  },
  {
    "resource_type": "aws_cloudhsm_v2_cluster",
    "node_type": "identity",
    "display_name": "AWS CloudHSM",
    "category": "Cryptography & PKI",
    "documentation_url": "https://docs.aws.amazon.com/cloudhsm/"
  },
  {
    "resource_type": "aws_kms_key",
    "node_type": "identity",
    "display_name": "AWS Key Management Service",
    "category": "Cryptography & PKI",
    "documentation_url": "https://docs.aws.amazon.com/kms/"
  },
  {
    "resource_type": "aws_acmpca_certificate_authority",
    "node_type": "identity",
    "display_name": "AWS Private Certificate Authority",
    "category": "Cryptography & PKI",
    "documentation_url": "https://docs.aws.amazon.com/privateca/"
  },
  {
    "resource_type": "aws_service:signer",
    "node_type": "external_dependency",
    "display_name": "AWS Signer",
    "category": "Cryptography & PKI",
    "documentation_url": "https://docs.aws.amazon.com/signer/"
  },
  {
    "resource_type": "aws_rds_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon Aurora",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/rds/"
  },
  {
    "resource_type": "aws_dsql_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon Aurora DSQL",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/aurora-dsql/"
  },
  {
    "resource_type": "aws_docdb_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon DocumentDB (with MongoDB compatibility)",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/documentdb/"
  },
  {
    "resource_type": "aws_dynamodb_table",
    "node_type": "managed_database",
    "display_name": "Amazon DynamoDB",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/dynamodb/"
  },
  {
    "resource_type": "aws_elasticache_replication_group",
    "node_type": "cache",
    "display_name": "Amazon ElastiCache",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/elasticache/"
  },
  {
    "resource_type": "aws_keyspaces_table",
    "node_type": "managed_database",
    "display_name": "Amazon Keyspaces (for Apache Cassandra)",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/keyspaces/"
  },
  {
    "resource_type": "aws_memorydb_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon MemoryDB",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/memorydb/"
  },
  {
    "resource_type": "aws_neptune_cluster",
    "node_type": "managed_database",
    "display_name": "Amazon Neptune",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/neptune/"
  },
  {
    "resource_type": "aws_service:oracle-database",
    "node_type": "managed_database",
    "display_name": "Oracle Database@AWS",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/odb/"
  },
  {
    "resource_type": "aws_db_instance",
    "node_type": "managed_database",
    "display_name": "Amazon Relational Database Service",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/rds/"
  },
  {
    "resource_type": "aws_timestreamwrite_table",
    "node_type": "managed_database",
    "display_name": "Amazon Timestream",
    "category": "Database",
    "documentation_url": "https://docs.aws.amazon.com/timestream/"
  },
  {
    "resource_type": "aws_service:cloud-control-api",
    "node_type": "external_dependency",
    "display_name": "AWS Cloud Control API",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/cloudcontrolapi/"
  },
  {
    "resource_type": "aws_service:cloud9",
    "node_type": "external_dependency",
    "display_name": "AWS Cloud9",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/cloud9/"
  },
  {
    "resource_type": "aws_service:cloudshell",
    "node_type": "external_dependency",
    "display_name": "AWS CloudShell",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/cloudshell/"
  },
  {
    "resource_type": "aws_service:codeartifact",
    "node_type": "external_dependency",
    "display_name": "AWS CodeArtifact",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/codeartifact/"
  },
  {
    "resource_type": "aws_service:codebuild",
    "node_type": "external_dependency",
    "display_name": "AWS CodeBuild",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/codebuild/"
  },
  {
    "resource_type": "aws_service:codecatalyst",
    "node_type": "external_dependency",
    "display_name": "Amazon CodeCatalyst",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/codecatalyst"
  },
  {
    "resource_type": "aws_service:codedeploy",
    "node_type": "external_dependency",
    "display_name": "AWS CodeDeploy",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/codedeploy/"
  },
  {
    "resource_type": "aws_service:codepipeline",
    "node_type": "external_dependency",
    "display_name": "AWS CodePipeline",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/codepipeline/"
  },
  {
    "resource_type": "aws_service:fault-injection-service",
    "node_type": "external_dependency",
    "display_name": "AWS Fault Injection Service",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/fis/"
  },
  {
    "resource_type": "aws_service:infrastructure-composer",
    "node_type": "external_dependency",
    "display_name": "AWS Infrastructure Composer",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/infrastructure-composer/"
  },
  {
    "resource_type": "aws_service:x-ray",
    "node_type": "external_dependency",
    "display_name": "AWS X-Ray",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/xray/"
  },
  {
    "resource_type": "aws_service:cloud-development-kit-aws-cdk",
    "node_type": "external_dependency",
    "display_name": "AWS Cloud Development Kit (AWS CDK)",
    "category": "Developer Tools",
    "documentation_url": "https://docs.aws.amazon.com/cdk/"
  },
  {
    "resource_type": "aws_workspaces_workspace",
    "node_type": "compute",
    "display_name": "Amazon WorkSpaces",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/workspaces/"
  },
  {
    "resource_type": "aws_appstream_fleet",
    "node_type": "compute",
    "display_name": "Amazon WorkSpaces Applications",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/appstream2/"
  },
  {
    "resource_type": "aws_service:workspaces-core",
    "node_type": "external_dependency",
    "display_name": "Amazon WorkSpaces Core",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/workspaces-core/"
  },
  {
    "resource_type": "aws_service:workspaces-thin-client",
    "node_type": "external_dependency",
    "display_name": "Amazon WorkSpaces Thin Client",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/workspaces-thin-client/"
  },
  {
    "resource_type": "aws_service:dcv",
    "node_type": "external_dependency",
    "display_name": "Amazon DCV",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/dcv/"
  },
  {
    "resource_type": "aws_service:workspaces-secure-browser",
    "node_type": "external_dependency",
    "display_name": "Amazon WorkSpaces Secure Browser",
    "category": "End User Computing",
    "documentation_url": "https://docs.aws.amazon.com/workspaces-web/"
  },
  {
    "resource_type": "aws_amplify_app",
    "node_type": "compute",
    "display_name": "AWS Amplify",
    "category": "Front-End Web & Mobile",
    "documentation_url": "https://docs.aws.amazon.com/amplify/"
  },
  {
    "resource_type": "aws_service:appsync",
    "node_type": "external_dependency",
    "display_name": "AWS AppSync",
    "category": "Front-End Web & Mobile",
    "documentation_url": "https://docs.aws.amazon.com/appsync/"
  },
  {
    "resource_type": "aws_service:device-farm",
    "node_type": "external_dependency",
    "display_name": "AWS Device Farm",
    "category": "Front-End Web & Mobile",
    "documentation_url": "https://docs.aws.amazon.com/devicefarm/"
  },
  {
    "resource_type": "aws_service:events-api",
    "node_type": "external_dependency",
    "display_name": "AWS Events API",
    "category": "Front-End Web & Mobile",
    "documentation_url": "https://docs.aws.amazon.com/events/"
  },
  {
    "resource_type": "aws_service:location-service",
    "node_type": "external_dependency",
    "display_name": "Amazon Location Service",
    "category": "Front-End Web & Mobile",
    "documentation_url": "https://docs.aws.amazon.com/location/"
  },
  {
    "resource_type": "aws_gamelift_fleet",
    "node_type": "compute",
    "display_name": "Amazon GameLift Servers",
    "category": "Game Development",
    "documentation_url": "https://docs.aws.amazon.com/gameliftservers/"
  },
  {
    "resource_type": "aws_service:gamelift-streams",
    "node_type": "external_dependency",
    "display_name": "Amazon GameLift Streams",
    "category": "Game Development",
    "documentation_url": "https://docs.aws.amazon.com/gameliftstreams/"
  },
  {
    "resource_type": "aws_service:freertos",
    "node_type": "external_dependency",
    "display_name": "FreeRTOS",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/freertos/"
  },
  {
    "resource_type": "aws_service:iot-core",
    "node_type": "external_dependency",
    "display_name": "AWS IoT Core",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot/"
  },
  {
    "resource_type": "aws_service:iot-device-defender",
    "node_type": "external_dependency",
    "display_name": "AWS IoT Device Defender",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-device-defender/"
  },
  {
    "resource_type": "aws_service:iot-device-management",
    "node_type": "external_dependency",
    "display_name": "AWS IoT Device Management",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-device-management/"
  },
  {
    "resource_type": "aws_service:iot-expresslink",
    "node_type": "external_dependency",
    "display_name": "AWS IoT ExpressLink",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-expresslink/"
  },
  {
    "resource_type": "aws_service:iot-fleetwise",
    "node_type": "external_dependency",
    "display_name": "AWS IoT FleetWise",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-fleetwise/"
  },
  {
    "resource_type": "aws_service:iot-greengrass",
    "node_type": "external_dependency",
    "display_name": "AWS IoT Greengrass",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/greengrass/"
  },
  {
    "resource_type": "aws_service:iot-sitewise",
    "node_type": "external_dependency",
    "display_name": "AWS IoT SiteWise",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-sitewise/"
  },
  {
    "resource_type": "aws_service:iot-twinmaker",
    "node_type": "external_dependency",
    "display_name": "AWS IoT TwinMaker",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-twinmaker/"
  },
  {
    "resource_type": "aws_service:iot-wireless",
    "node_type": "external_dependency",
    "display_name": "AWS IoT Wireless",
    "category": "Internet of Things (IoT)",
    "documentation_url": "https://docs.aws.amazon.com/iot-wireless/"
  },
  {
    "resource_type": "aws_service:a2i",
    "node_type": "external_dependency",
    "display_name": "Amazon A2I",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/augmented-ai/"
  },
  {
    "resource_type": "aws_service:bedrock",
    "node_type": "external_dependency",
    "display_name": "Amazon Bedrock",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/bedrock/"
  },
  {
    "resource_type": "aws_service:bedrock-agentcore",
    "node_type": "external_dependency",
    "display_name": "Amazon Bedrock AgentCore",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/bedrock-agentcore/"
  },
  {
    "resource_type": "aws_service:claude-platform-on-aws",
    "node_type": "external_dependency",
    "display_name": "Claude Platform on AWS",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/claude-platform/"
  },
  {
    "resource_type": "aws_service:codeguru",
    "node_type": "external_dependency",
    "display_name": "Amazon CodeGuru",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/codeguru/"
  },
  {
    "resource_type": "aws_service:comprehend",
    "node_type": "external_dependency",
    "display_name": "Amazon Comprehend",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/comprehend/"
  },
  {
    "resource_type": "aws_service:comprehend-medical",
    "node_type": "external_dependency",
    "display_name": "Amazon Comprehend Medical",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/comprehend-medical/"
  },
  {
    "resource_type": "aws_service:deep-learning-amis",
    "node_type": "external_dependency",
    "display_name": "AWS Deep Learning AMIs",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/dlami/"
  },
  {
    "resource_type": "aws_service:deep-learning-containers",
    "node_type": "external_dependency",
    "display_name": "AWS Deep Learning Containers",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/deep-learning-containers/"
  },
  {
    "resource_type": "aws_service:devops-guru",
    "node_type": "external_dependency",
    "display_name": "Amazon DevOps Guru",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/devops-guru/"
  },
  {
    "resource_type": "aws_service:forecast",
    "node_type": "external_dependency",
    "display_name": "Amazon Forecast",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/forecast/"
  },
  {
    "resource_type": "aws_service:fraud-detector",
    "node_type": "external_dependency",
    "display_name": "Amazon Fraud Detector",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/frauddetector/"
  },
  {
    "resource_type": "aws_service:healthimaging",
    "node_type": "external_dependency",
    "display_name": "AWS HealthImaging",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/healthimaging/"
  },
  {
    "resource_type": "aws_service:healthlake",
    "node_type": "external_dependency",
    "display_name": "AWS HealthLake",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/healthlake/"
  },
  {
    "resource_type": "aws_service:healthomics",
    "node_type": "external_dependency",
    "display_name": "AWS HealthOmics",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/omics/"
  },
  {
    "resource_type": "aws_service:kendra",
    "node_type": "external_dependency",
    "display_name": "Amazon Kendra",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/kendra/"
  },
  {
    "resource_type": "aws_service:lex",
    "node_type": "external_dependency",
    "display_name": "Amazon Lex",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/lex/"
  },
  {
    "resource_type": "aws_service:monitron",
    "node_type": "external_dependency",
    "display_name": "Amazon Monitron",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/Monitron/"
  },
  {
    "resource_type": "aws_service:nova",
    "node_type": "external_dependency",
    "display_name": "Amazon Nova",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/nova/"
  },
  {
    "resource_type": "aws_service:nova-act",
    "node_type": "external_dependency",
    "display_name": "Amazon Nova Act",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/nova-act/"
  },
  {
    "resource_type": "aws_service:panorama",
    "node_type": "external_dependency",
    "display_name": "AWS Panorama",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/panorama/"
  },
  {
    "resource_type": "aws_service:personalize",
    "node_type": "external_dependency",
    "display_name": "Amazon Personalize",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/personalize/"
  },
  {
    "resource_type": "aws_service:polly",
    "node_type": "external_dependency",
    "display_name": "Amazon Polly",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/polly/"
  },
  {
    "resource_type": "aws_service:rekognition",
    "node_type": "external_dependency",
    "display_name": "Amazon Rekognition",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/rekognition/"
  },
  {
    "resource_type": "aws_service:sagemaker-ai",
    "node_type": "external_dependency",
    "display_name": "Amazon SageMaker AI",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/sagemaker/"
  },
  {
    "resource_type": "aws_service:textract",
    "node_type": "external_dependency",
    "display_name": "Amazon Textract",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/textract/"
  },
  {
    "resource_type": "aws_service:transcribe",
    "node_type": "external_dependency",
    "display_name": "Amazon Transcribe",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/transcribe/"
  },
  {
    "resource_type": "aws_service:translate",
    "node_type": "external_dependency",
    "display_name": "Amazon Translate",
    "category": "Machine Learning",
    "documentation_url": "https://docs.aws.amazon.com/translate/"
  },
  {
    "resource_type": "aws_service:account-management",
    "node_type": "external_dependency",
    "display_name": "AWS Account Management",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/organizations/"
  },
  {
    "resource_type": "aws_service:appconfig",
    "node_type": "external_dependency",
    "display_name": "AWS AppConfig",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/appconfig/"
  },
  {
    "resource_type": "aws_service:cloudformation",
    "node_type": "external_dependency",
    "display_name": "AWS CloudFormation",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/cloudformation/"
  },
  {
    "resource_type": "aws_service:cloudtrail",
    "node_type": "external_dependency",
    "display_name": "AWS CloudTrail",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/cloudtrail/"
  },
  {
    "resource_type": "aws_service:q-developer-in-chat-applications",
    "node_type": "external_dependency",
    "display_name": "Amazon Q Developer in chat applications",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/chatbot/"
  },
  {
    "resource_type": "aws_service:cloudwatch",
    "node_type": "external_dependency",
    "display_name": "Amazon CloudWatch",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/cloudwatch/"
  },
  {
    "resource_type": "aws_autoscaling_group",
    "node_type": "compute",
    "display_name": "Amazon EC2 Auto Scaling",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/autoscaling/"
  },
  {
    "resource_type": "aws_service:security-agent",
    "node_type": "external_dependency",
    "display_name": "AWS Security Agent",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/securityagent/"
  },
  {
    "resource_type": "aws_service:compute-optimizer",
    "node_type": "external_dependency",
    "display_name": "AWS Compute Optimizer",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/compute-optimizer/"
  },
  {
    "resource_type": "aws_service:config",
    "node_type": "external_dependency",
    "display_name": "AWS Config",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/config/"
  },
  {
    "resource_type": "aws_service:control-tower",
    "node_type": "external_dependency",
    "display_name": "AWS Control Tower",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/controltower/"
  },
  {
    "resource_type": "aws_service:data-lifecycle-manager",
    "node_type": "external_dependency",
    "display_name": "Amazon Data Lifecycle Manager",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/dlm/"
  },
  {
    "resource_type": "aws_service:devops-agent",
    "node_type": "external_dependency",
    "display_name": "AWS DevOps Agent",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/devopsagent/"
  },
  {
    "resource_type": "aws_service:health",
    "node_type": "external_dependency",
    "display_name": "AWS Health",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/health/"
  },
  {
    "resource_type": "aws_service:launch-wizard",
    "node_type": "external_dependency",
    "display_name": "AWS Launch Wizard",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/launchwizard/"
  },
  {
    "resource_type": "aws_service:license-manager",
    "node_type": "external_dependency",
    "display_name": "AWS License Manager",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/license-manager/"
  },
  {
    "resource_type": "aws_service:managed-grafana",
    "node_type": "external_dependency",
    "display_name": "Amazon Managed Grafana",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/grafana/"
  },
  {
    "resource_type": "aws_service:managed-service-for-prometheus",
    "node_type": "external_dependency",
    "display_name": "Amazon Managed Service for Prometheus",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/prometheus/"
  },
  {
    "resource_type": "aws_service:organizations",
    "node_type": "external_dependency",
    "display_name": "AWS Organizations",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/organizations/"
  },
  {
    "resource_type": "aws_service:proton",
    "node_type": "external_dependency",
    "display_name": "AWS Proton",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/proton/"
  },
  {
    "resource_type": "aws_service:resilience-hub",
    "node_type": "external_dependency",
    "display_name": "AWS Resilience Hub",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/resilience-hub/"
  },
  {
    "resource_type": "aws_service:resource-explorer",
    "node_type": "external_dependency",
    "display_name": "AWS Resource Explorer",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/ARG/"
  },
  {
    "resource_type": "aws_service:resource-groups",
    "node_type": "external_dependency",
    "display_name": "AWS Resource Groups",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/ARG/"
  },
  {
    "resource_type": "aws_service:service-catalog",
    "node_type": "external_dependency",
    "display_name": "AWS Service Catalog",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/servicecatalog/"
  },
  {
    "resource_type": "aws_service:service-management-connector",
    "node_type": "external_dependency",
    "display_name": "AWS Service Management Connector",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/smc/"
  },
  {
    "resource_type": "aws_service:service-quotas",
    "node_type": "external_dependency",
    "display_name": "Service Quotas",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/servicequotas/"
  },
  {
    "resource_type": "aws_service:sustainability",
    "node_type": "external_dependency",
    "display_name": "AWS Sustainability",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/sustainability/"
  },
  {
    "resource_type": "aws_service:systems-manager",
    "node_type": "external_dependency",
    "display_name": "AWS Systems Manager",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/systems-manager/"
  },
  {
    "resource_type": "aws_service:systems-manager-incident-manager",
    "node_type": "external_dependency",
    "display_name": "AWS Systems Manager Incident Manager",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/systems-manager/"
  },
  {
    "resource_type": "aws_service:tag-editor",
    "node_type": "external_dependency",
    "display_name": "Tag Editor",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/ARG/"
  },
  {
    "resource_type": "aws_service:telco-network-builder",
    "node_type": "external_dependency",
    "display_name": "AWS Telco Network Builder",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/tnb/"
  },
  {
    "resource_type": "aws_service:trusted-advisor",
    "node_type": "external_dependency",
    "display_name": "AWS Trusted Advisor",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/aws-support/"
  },
  {
    "resource_type": "aws_service:user-notifications",
    "node_type": "external_dependency",
    "display_name": "AWS User Notifications",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/notifications/"
  },
  {
    "resource_type": "aws_service:well-architected-tool",
    "node_type": "external_dependency",
    "display_name": "AWS Well-Architected Tool",
    "category": "Management & Governance",
    "documentation_url": "https://docs.aws.amazon.com/wellarchitected/"
  },
  {
    "resource_type": "aws_deadline_farm",
    "node_type": "compute",
    "display_name": "AWS Deadline Cloud",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/deadline-cloud/"
  },
  {
    "resource_type": "aws_service:elemental-inference",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental Inference",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/medialive/"
  },
  {
    "resource_type": "aws_service:elemental-mediaconnect",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental MediaConnect",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/mediaconnect/"
  },
  {
    "resource_type": "aws_service:elemental-mediaconvert",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental MediaConvert",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/mediaconvert/"
  },
  {
    "resource_type": "aws_service:elemental-medialive",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental MediaLive",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/medialive/"
  },
  {
    "resource_type": "aws_service:elemental-mediapackage",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental MediaPackage",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/mediapackage/"
  },
  {
    "resource_type": "aws_service:elemental-mediatailor",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental MediaTailor",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/mediatailor/"
  },
  {
    "resource_type": "aws_service:elemental-on-premises",
    "node_type": "external_dependency",
    "display_name": "AWS Elemental On-Premises",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/elemental-on-premises/"
  },
  {
    "resource_type": "aws_service:interactive-video-service",
    "node_type": "external_dependency",
    "display_name": "Amazon Interactive Video Service",
    "category": "Media Services",
    "documentation_url": "https://docs.aws.amazon.com/ivs/"
  },
  {
    "resource_type": "aws_service:application-discovery-service",
    "node_type": "external_dependency",
    "display_name": "AWS Application Discovery Service",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/application-discovery/"
  },
  {
    "resource_type": "aws_service:data-transfer-terminal",
    "node_type": "external_dependency",
    "display_name": "AWS Data Transfer Terminal",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/datatransferterminal/"
  },
  {
    "resource_type": "aws_service:database-migration-service",
    "node_type": "external_dependency",
    "display_name": "AWS Database Migration Service",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/dms/"
  },
  {
    "resource_type": "aws_service:datasync",
    "node_type": "external_dependency",
    "display_name": "AWS DataSync",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/datasync/"
  },
  {
    "resource_type": "aws_service:elastic-vmware-service",
    "node_type": "external_dependency",
    "display_name": "Amazon Elastic VMware Service",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/evs/"
  },
  {
    "resource_type": "aws_service:mainframe-modernization",
    "node_type": "external_dependency",
    "display_name": "AWS Mainframe Modernization",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/m2/"
  },
  {
    "resource_type": "aws_service:migration-hub",
    "node_type": "external_dependency",
    "display_name": "AWS Migration Hub",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/migrationhub/"
  },
  {
    "resource_type": "aws_service:transfer-family",
    "node_type": "external_dependency",
    "display_name": "AWS Transfer Family",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/transfer/"
  },
  {
    "resource_type": "aws_service:transform",
    "node_type": "external_dependency",
    "display_name": "AWS Transform",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/transform/"
  },
  {
    "resource_type": "aws_service:transform-mgn",
    "node_type": "external_dependency",
    "display_name": "AWS Transform MGN",
    "category": "Migration & Transfer",
    "documentation_url": "https://docs.aws.amazon.com/mgn/"
  },
  {
    "resource_type": "aws_api_gateway_rest_api",
    "node_type": "load_balancer",
    "display_name": "Amazon API Gateway",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/apigateway/"
  },
  {
    "resource_type": "aws_appmesh_mesh",
    "node_type": "network_boundary",
    "display_name": "AWS App Mesh",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/app-mesh/"
  },
  {
    "resource_type": "aws_service_discovery_service",
    "node_type": "dns",
    "display_name": "AWS Cloud Map",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/cloud-map/"
  },
  {
    "resource_type": "aws_cloudfront_distribution",
    "node_type": "load_balancer",
    "display_name": "Amazon CloudFront",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/cloudfront/"
  },
  {
    "resource_type": "aws_dx_connection",
    "node_type": "network_boundary",
    "display_name": "AWS Direct Connect",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/directconnect/"
  },
  {
    "resource_type": "aws_lb",
    "node_type": "load_balancer",
    "display_name": "Elastic Load Balancing",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/elasticloadbalancing/"
  },
  {
    "resource_type": "aws_route53_record",
    "node_type": "dns",
    "display_name": "Amazon Route 53",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/route53/"
  },
  {
    "resource_type": "aws_service:application-recovery-controller-arc",
    "node_type": "external_dependency",
    "display_name": "Amazon Application Recovery Controller (ARC)",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/amazonarc/"
  },
  {
    "resource_type": "aws_globalaccelerator_accelerator",
    "node_type": "load_balancer",
    "display_name": "AWS Global Accelerator",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/global-accelerator/"
  },
  {
    "resource_type": "aws_service:interconnect",
    "node_type": "network_boundary",
    "display_name": "AWS Interconnect",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/interconnect/"
  },
  {
    "resource_type": "aws_service:network-security-manager",
    "node_type": "network_boundary",
    "display_name": "AWS Network Security Manager",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/network-security-manager/"
  },
  {
    "resource_type": "aws_service:rtb-fabric",
    "node_type": "external_dependency",
    "display_name": "AWS RTB Fabric",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/rtb-fabric/"
  },
  {
    "resource_type": "aws_verifiedaccess_instance",
    "node_type": "network_boundary",
    "display_name": "AWS Verified Access",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/verified-access/"
  },
  {
    "resource_type": "aws_vpc",
    "node_type": "network_boundary",
    "display_name": "Amazon Virtual Private Cloud",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/vpc/"
  },
  {
    "resource_type": "aws_vpn_connection",
    "node_type": "network_boundary",
    "display_name": "AWS Virtual Private Network",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/vpn/"
  },
  {
    "resource_type": "aws_vpclattice_service",
    "node_type": "network_boundary",
    "display_name": "Amazon VPC Lattice",
    "category": "Networking & Content Delivery",
    "documentation_url": "https://docs.aws.amazon.com/vpc-lattice/"
  },
  {
    "resource_type": "aws_service:braket",
    "node_type": "external_dependency",
    "display_name": "Amazon Braket",
    "category": "Quantum Computing",
    "documentation_url": "https://docs.aws.amazon.com/braket/"
  },
  {
    "resource_type": "aws_service:ground-station",
    "node_type": "external_dependency",
    "display_name": "AWS Ground Station",
    "category": "Satellite",
    "documentation_url": "https://docs.aws.amazon.com/ground-station/"
  },
  {
    "resource_type": "aws_service:artifact",
    "node_type": "external_dependency",
    "display_name": "AWS Artifact",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/artifact/"
  },
  {
    "resource_type": "aws_cognito_user_pool",
    "node_type": "identity",
    "display_name": "Amazon Cognito",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/cognito/"
  },
  {
    "resource_type": "aws_directory_service_directory",
    "node_type": "identity",
    "display_name": "AWS Directory Service",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/directory-service/"
  },
  {
    "resource_type": "aws_service:guardduty",
    "node_type": "external_dependency",
    "display_name": "Amazon GuardDuty",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/guardduty/"
  },
  {
    "resource_type": "aws_iam_role",
    "node_type": "identity",
    "display_name": "AWS Identity and Access Management",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/iam/"
  },
  {
    "resource_type": "aws_service:audit-manager",
    "node_type": "external_dependency",
    "display_name": "AWS Audit Manager",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/audit-manager/"
  },
  {
    "resource_type": "aws_cloud_directory_directory",
    "node_type": "identity",
    "display_name": "Amazon Cloud Directory",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/clouddirectory/"
  },
  {
    "resource_type": "aws_service:detective",
    "node_type": "external_dependency",
    "display_name": "Amazon Detective",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/detective/"
  },
  {
    "resource_type": "aws_fms_policy",
    "node_type": "network_boundary",
    "display_name": "AWS Firewall Manager",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/firewall-manager/"
  },
  {
    "resource_type": "aws_ssoadmin_permission_set",
    "node_type": "identity",
    "display_name": "AWS IAM Identity Center",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/singlesignon/"
  },
  {
    "resource_type": "aws_service:inspector",
    "node_type": "external_dependency",
    "display_name": "Amazon Inspector",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/inspector/"
  },
  {
    "resource_type": "aws_service:macie",
    "node_type": "external_dependency",
    "display_name": "Amazon Macie",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/macie/"
  },
  {
    "resource_type": "aws_networkfirewall_firewall",
    "node_type": "network_boundary",
    "display_name": "AWS Network Firewall",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/network-firewall/"
  },
  {
    "resource_type": "aws_service:payment-cryptography",
    "node_type": "external_dependency",
    "display_name": "AWS Payment Cryptography",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/payment-cryptography/"
  },
  {
    "resource_type": "aws_ram_resource_share",
    "node_type": "identity",
    "display_name": "AWS Resource Access Manager",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/ARG/"
  },
  {
    "resource_type": "aws_secretsmanager_secret",
    "node_type": "identity",
    "display_name": "AWS Secrets Manager",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/secretsmanager/"
  },
  {
    "resource_type": "aws_service:security-hub",
    "node_type": "external_dependency",
    "display_name": "AWS Security Hub",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/securityhub/"
  },
  {
    "resource_type": "aws_service:security-incident-response",
    "node_type": "external_dependency",
    "display_name": "AWS Security Incident Response",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/security-ir/"
  },
  {
    "resource_type": "aws_service:security-lake",
    "node_type": "external_dependency",
    "display_name": "Amazon Security Lake",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/security-lake/"
  },
  {
    "resource_type": "aws_shield_protection",
    "node_type": "network_boundary",
    "display_name": "AWS Shield",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/shield/"
  },
  {
    "resource_type": "aws_verifiedpermissions_policy_store",
    "node_type": "identity",
    "display_name": "Amazon Verified Permissions",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/verifiedpermissions/"
  },
  {
    "resource_type": "aws_wafv2_web_acl",
    "node_type": "network_boundary",
    "display_name": "AWS WAF",
    "category": "Security, Identity, & Compliance",
    "documentation_url": "https://docs.aws.amazon.com/waf/"
  },
  {
    "resource_type": "aws_service:fargate",
    "node_type": "container_workload",
    "display_name": "AWS Fargate",
    "category": "Serverless",
    "documentation_url": "https://docs.aws.amazon.com/AmazonECS/latest/developerguide/AWS_Fargate.html"
  },
  {
    "resource_type": "aws_s3_bucket",
    "node_type": "object_store",
    "display_name": "Amazon Simple Storage Service",
    "category": "Serverless",
    "documentation_url": "https://docs.aws.amazon.com/s3/"
  },
  {
    "resource_type": "aws_service:backup",
    "node_type": "external_dependency",
    "display_name": "AWS Backup",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/aws-backup/"
  },
  {
    "resource_type": "aws_service:elastic-block-store",
    "node_type": "external_dependency",
    "display_name": "Amazon Elastic Block Store",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/ebs/"
  },
  {
    "resource_type": "aws_service:elastic-disaster-recovery",
    "node_type": "external_dependency",
    "display_name": "AWS Elastic Disaster Recovery",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/drs/"
  },
  {
    "resource_type": "aws_service:elastic-file-system",
    "node_type": "external_dependency",
    "display_name": "Amazon Elastic File System",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/efs/"
  },
  {
    "resource_type": "aws_service:fsx",
    "node_type": "external_dependency",
    "display_name": "Amazon FSx",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/fsx/"
  },
  {
    "resource_type": "aws_glacier_vault",
    "node_type": "object_store",
    "display_name": "Amazon Glacier",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/s3/"
  },
  {
    "resource_type": "aws_service:snowball-edge",
    "node_type": "external_dependency",
    "display_name": "AWS Snowball Edge",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/snowball/"
  },
  {
    "resource_type": "aws_service:storage-gateway",
    "node_type": "external_dependency",
    "display_name": "AWS Storage Gateway",
    "category": "Storage",
    "documentation_url": "https://docs.aws.amazon.com/storagegateway/"
  },
  {
    "resource_type": "aws_route53_zone",
    "node_type": "dns",
    "display_name": "Route 53 hosted zone",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_route53_resolver_endpoint",
    "node_type": "dns",
    "display_name": "Route 53 Resolver",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_ec2_transit_gateway",
    "node_type": "network_boundary",
    "display_name": "Transit Gateway",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_vpc_endpoint",
    "node_type": "network_boundary",
    "display_name": "AWS PrivateLink",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_vpc_peering_connection",
    "node_type": "network_boundary",
    "display_name": "VPC peering",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_networkmanager_core_network",
    "node_type": "network_boundary",
    "display_name": "AWS Cloud WAN",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_egress_only_internet_gateway",
    "node_type": "network_boundary",
    "display_name": "Egress-only internet gateway",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_elasticache_serverless_cache",
    "node_type": "cache",
    "display_name": "ElastiCache Serverless",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_dynamodb_global_table",
    "node_type": "managed_database",
    "display_name": "DynamoDB global table",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_ecs_task_definition",
    "node_type": "container_workload",
    "display_name": "ECS task",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  },
  {
    "resource_type": "aws_eks_node_group",
    "node_type": "container_workload",
    "display_name": "EKS node group",
    "category": "Infrastructure",
    "documentation_url": "https://docs.aws.amazon.com/"
  }
];

