output "sns_topic_arn" {
  description = "SNS topic ARN for order-created events."
  value       = aws_sns_topic.order_processing_events.arn
}

output "sqs_queue_url" {
  description = "SQS queue URL consumed by the order processor."
  value       = aws_sqs_queue.order_processing_queue.id
}

output "sqs_queue_arn" {
  description = "SQS queue ARN."
  value       = aws_sqs_queue.order_processing_queue.arn
}

output "api_publish_policy_arn" {
  description = "IAM policy ARN granting sns:Publish to the API task."
  value       = aws_iam_policy.api_publish.arn
}

output "processor_consume_policy_arn" {
  description = "IAM policy ARN granting SQS consume permissions to the processor task."
  value       = aws_iam_policy.processor_consume.arn
}

output "api_environment" {
  description = "Environment values the API task needs for AWS messaging mode."
  value = {
    MESSAGING_BACKEND = "aws"
    AWS_REGION        = var.aws_region
    SNS_TOPIC_ARN     = aws_sns_topic.order_processing_events.arn
  }
}

output "processor_environment" {
  description = "Environment values the processor task needs for AWS messaging mode."
  value = {
    MESSAGING_BACKEND      = "aws"
    AWS_REGION             = var.aws_region
    SQS_QUEUE_URL          = aws_sqs_queue.order_processing_queue.id
    SQS_WAIT_TIME_SECONDS  = var.queue_receive_wait_time_seconds
    PAYMENT_DELAY_SECONDS  = 3
    PROCESSOR_WORKER_COUNT = 1
  }
}
