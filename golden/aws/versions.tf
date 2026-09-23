# Golden reference architecture - AWS side. See CLAUDE.md §14.
# Authored to stay inside the §15 v1 parser subset: resource/data blocks, variables,
# locals and static references only. No count, for_each, dynamic blocks, or computed
# interpolation - this bundle is the fixture the parser is measured against, so anything
# it contains must be something the defined subset can actually resolve.
terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}
