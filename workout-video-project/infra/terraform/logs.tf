resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.project_name}/api"
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-api-logs"
  })
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/${var.project_name}/worker"
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-worker-logs"
  })
}

resource "aws_cloudwatch_log_group" "reaper" {
  name              = "/ecs/${var.project_name}/reaper"
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-reaper-logs"
  })
}
