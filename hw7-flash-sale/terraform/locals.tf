data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_iam_role" "ecs_task_role" {
  name = var.ecs_task_role_name
}

locals {
  selected_azs = slice(data.aws_availability_zones.available.names, 0, 2)

  api_container_name       = "${var.project_name}-api"
  processor_container_name = "${var.project_name}-processor"
  lambda_function_name     = "${var.project_name}-lambda-processor"

  api_log_group_name       = "/ecs/${var.project_name}/api"
  processor_log_group_name = "/ecs/${var.project_name}/processor"
  lambda_log_group_name    = "/aws/lambda/${local.lambda_function_name}"

  api_image_uri       = "${aws_ecr_repository.api.repository_url}:${var.api_image_tag}"
  processor_image_uri = "${aws_ecr_repository.processor.repository_url}:${var.processor_image_tag}"

  api_environment = [
    { name = "PORT", value = tostring(var.container_port) },
    { name = "GIN_MODE", value = "release" },
    { name = "MESSAGING_BACKEND", value = "aws" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "SNS_TOPIC_ARN", value = aws_sns_topic.order_processing_events.arn },
    { name = "PAYMENT_DELAY_SECONDS", value = tostring(var.payment_delay_seconds) },
    { name = "SYNC_PAYMENT_SLOTS", value = tostring(var.sync_payment_slots) },
  ]

  processor_environment = [
    { name = "MESSAGING_BACKEND", value = "aws" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "SQS_QUEUE_URL", value = aws_sqs_queue.order_processing_queue.id },
    { name = "SQS_WAIT_TIME_SECONDS", value = tostring(var.queue_receive_wait_time_seconds) },
    { name = "PAYMENT_DELAY_SECONDS", value = tostring(var.payment_delay_seconds) },
    { name = "PROCESSOR_WORKER_COUNT", value = tostring(var.processor_worker_count) },
  ]
}
