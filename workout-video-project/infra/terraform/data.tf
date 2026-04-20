data "aws_vpc" "default" {
  count   = var.vpc_id == "" ? 1 : 0
  default = true
}

data "aws_caller_identity" "current" {}

data "aws_subnets" "selected" {
  count = length(var.subnet_ids) == 0 ? 1 : 0

  filter {
    name   = "vpc-id"
    values = [local.effective_vpc_id]
  }
}
