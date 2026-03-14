locals {
  resource_tags = merge(var.common_tags, {
    ManagedBy = "Terraform"
    Project   = var.project_name
  })
}

resource "aws_sns_topic" "order_processing_events" {
  name = var.sns_topic_name
  tags = merge(local.resource_tags, {
    Name = var.sns_topic_name
  })
}

resource "aws_sqs_queue" "order_processing_queue" {
  name                       = var.sqs_queue_name
  visibility_timeout_seconds = var.queue_visibility_timeout_seconds
  message_retention_seconds  = var.queue_message_retention_seconds
  receive_wait_time_seconds  = var.queue_receive_wait_time_seconds

  tags = merge(local.resource_tags, {
    Name = var.sqs_queue_name
  })
}

data "aws_iam_policy_document" "order_processing_queue" {
  statement {
    sid    = "AllowSNSPublishToQueue"
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["sns.amazonaws.com"]
    }

    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.order_processing_queue.arn]

    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = [aws_sns_topic.order_processing_events.arn]
    }
  }
}

resource "aws_sqs_queue_policy" "order_processing_queue" {
  queue_url = aws_sqs_queue.order_processing_queue.id
  policy    = data.aws_iam_policy_document.order_processing_queue.json
}

resource "aws_sns_topic_subscription" "order_processing_queue" {
  topic_arn            = aws_sns_topic.order_processing_events.arn
  protocol             = "sqs"
  endpoint             = aws_sqs_queue.order_processing_queue.arn
  raw_message_delivery = false
}

data "aws_iam_policy_document" "api_publish" {
  statement {
    sid    = "AllowPublishOrderEvents"
    effect = "Allow"

    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.order_processing_events.arn]
  }
}

resource "aws_iam_policy" "api_publish" {
  name        = "${var.project_name}-api-publish"
  description = "Allows the API task to publish order-created events to SNS."
  policy      = data.aws_iam_policy_document.api_publish.json
}

data "aws_iam_policy_document" "processor_consume" {
  statement {
    sid    = "AllowConsumeOrderQueue"
    effect = "Allow"

    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:ChangeMessageVisibility",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.order_processing_queue.arn]
  }
}

resource "aws_iam_policy" "processor_consume" {
  name        = "${var.project_name}-processor-consume"
  description = "Allows the processor task to poll and delete messages from the order queue."
  policy      = data.aws_iam_policy_document.processor_consume.json
}

data "aws_iam_role" "lab_role" {
  count = var.attach_policies_to_lab_role ? 1 : 0
  name  = var.lab_role_name
}

resource "aws_iam_role_policy_attachment" "lab_role_api_publish" {
  count      = var.attach_policies_to_lab_role ? 1 : 0
  role       = data.aws_iam_role.lab_role[0].name
  policy_arn = aws_iam_policy.api_publish.arn
}

resource "aws_iam_role_policy_attachment" "lab_role_processor_consume" {
  count      = var.attach_policies_to_lab_role ? 1 : 0
  role       = data.aws_iam_role.lab_role[0].name
  policy_arn = aws_iam_policy.processor_consume.arn
}
