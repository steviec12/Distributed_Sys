resource "aws_cloudwatch_log_group" "lambda_processor" {
  count = var.enable_lambda_subscriber ? 1 : 0

  name              = local.lambda_log_group_name
  retention_in_days = var.log_retention_days

  tags = merge(local.resource_tags, {
    Name = local.lambda_log_group_name
  })
}

resource "aws_lambda_function" "order_processor" {
  count = var.enable_lambda_subscriber ? 1 : 0

  function_name = local.lambda_function_name
  role          = data.aws_iam_role.ecs_task_role.arn
  runtime       = "provided.al2023"
  architectures = ["x86_64"]
  handler       = "bootstrap"

  filename         = "${path.module}/../dist/lambda/lambda.zip"
  source_code_hash = filebase64sha256("${path.module}/../dist/lambda/lambda.zip")

  timeout     = var.payment_delay_seconds + 5
  memory_size = 256

  environment {
    variables = {
      PAYMENT_DELAY_SECONDS = tostring(var.payment_delay_seconds)
    }
  }

  depends_on = [aws_cloudwatch_log_group.lambda_processor]

  tags = merge(local.resource_tags, {
    Name = local.lambda_function_name
  })
}

resource "aws_lambda_permission" "allow_sns" {
  count = var.enable_lambda_subscriber ? 1 : 0

  statement_id  = "AllowExecutionFromSNS"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.order_processor[0].function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.order_processing_events.arn
}

resource "aws_sns_topic_subscription" "lambda_order_processor" {
  count = var.enable_lambda_subscriber ? 1 : 0

  topic_arn = aws_sns_topic.order_processing_events.arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.order_processor[0].arn
}
