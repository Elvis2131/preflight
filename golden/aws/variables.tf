variable "region" {
  description = "Primary AWS region for the payments workload."
  type        = string
  default     = "eu-west-1"
}

variable "environment" {
  description = "Deployment environment. NOTE: whether this becomes a first-class dimension of workload.yaml is an open question per CLAUDE.md §20 - it is a plain tag here, nothing more."
  type        = string
  default     = "prod"
}

variable "service_name" {
  description = "Workload identifier, matches workload.yaml `name`."
  type        = string
  default     = "payments-api"
}

variable "vpc_cidr" {
  description = "CIDR for the payments VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "dns_zone_name" {
  description = "Public hosted zone for the payments API."
  type        = string
  default     = "payments.example.com"
}

variable "db_username" {
  description = "Master username for the payments database."
  type        = string
  default     = "payments_admin"
}

locals {
  # Three AZs. Written out explicitly rather than derived, per the §15 parser subset.
  az_a = "eu-west-1a"
  az_b = "eu-west-1b"
  az_c = "eu-west-1c"

  common_tags = {
    Service     = "payments-api"
    Tier        = "tier1"
    Compliance  = "PCI"
    ManagedBy   = "terraform"
  }
}
