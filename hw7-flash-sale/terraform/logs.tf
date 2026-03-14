resource "aws_cloudwatch_log_group" "api" {
  name              = local.api_log_group_name
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = local.api_log_group_name
  })
}

resource "aws_cloudwatch_log_group" "processor" {
  name              = local.processor_log_group_name
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = local.processor_log_group_name
  })
}
