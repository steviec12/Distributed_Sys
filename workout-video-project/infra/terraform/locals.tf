locals {
  resource_tags = merge({
    Project   = var.project_name
    ManagedBy = "Terraform"
    Service   = "workout-video"
  }, var.common_tags)

  effective_vpc_id = var.vpc_id != "" ? var.vpc_id : data.aws_vpc.default[0].id

  discovered_subnet_ids = try(sort(data.aws_subnets.selected[0].ids), [])

  effective_subnet_ids = length(var.subnet_ids) > 0 ? var.subnet_ids : (
    length(local.discovered_subnet_ids) >= 2
    ? slice(local.discovered_subnet_ids, 0, 2)
    : local.discovered_subnet_ids
  )

  learner_lab_role_arn = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/LabRole"

  api_container_name    = "api"
  worker_container_name = "worker"
  reaper_container_name = "reaper"

  redis_password_environment = var.redis_password != "" ? [
    { name = "REDIS_PASSWORD", value = var.redis_password }
  ] : []

  api_environment = concat([
    { name = "DEPLOYMENT_MODE", value = "aws" },
    { name = "PORT", value = tostring(var.api_container_port) },
    { name = "REDIS_ADDR", value = var.redis_endpoint },
    { name = "REDIS_DB", value = tostring(var.redis_db) },
    { name = "DYNAMO_ENABLED", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "DYNAMO_TABLE_NAME", value = var.dynamodb_table_name },
    { name = "S3_ENABLED", value = "true" },
    { name = "S3_BUCKET", value = var.s3_bucket_name },
    { name = "S3_PRESIGN_EXPIRATION_SECONDS", value = tostring(var.s3_presign_expiration_seconds) },
    { name = "MULTIPART_PART_SIZE_BYTES", value = tostring(var.multipart_part_size_bytes) },
  ], local.redis_password_environment)

  worker_environment = concat([
    { name = "DEPLOYMENT_MODE", value = "aws" },
    { name = "REDIS_ADDR", value = var.redis_endpoint },
    { name = "REDIS_DB", value = tostring(var.redis_db) },
    { name = "QUEUE_KEY", value = "queue:pending" },
    { name = "PROCESSING_QUEUE_KEY", value = "queue:processing" },
    { name = "RETRYABLE_FAILED_SET_KEY", value = "set:retryable_failed" },
    { name = "HEARTBEAT_INTERVAL_SECONDS", value = tostring(var.heartbeat_interval_seconds) },
    { name = "MAX_RETRIES", value = tostring(var.max_retries) },
    { name = "DYNAMO_ENABLED", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "DYNAMO_TABLE_NAME", value = var.dynamodb_table_name },
    { name = "S3_ENABLED", value = "true" },
    { name = "S3_BUCKET", value = var.s3_bucket_name },
  ], local.redis_password_environment)

  reaper_environment = concat([
    { name = "DEPLOYMENT_MODE", value = "aws" },
    { name = "REDIS_ADDR", value = var.redis_endpoint },
    { name = "REDIS_DB", value = tostring(var.redis_db) },
    { name = "QUEUE_KEY", value = "queue:pending" },
    { name = "PROCESSING_QUEUE_KEY", value = "queue:processing" },
    { name = "RETRYABLE_FAILED_SET_KEY", value = "set:retryable_failed" },
    { name = "REAPER_INTERVAL_SECONDS", value = tostring(var.reaper_interval_seconds) },
    { name = "STALE_TIMEOUT_SECONDS", value = tostring(var.stale_timeout_seconds) },
    { name = "MAX_RETRIES", value = tostring(var.max_retries) },
    { name = "DYNAMO_ENABLED", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "DYNAMO_TABLE_NAME", value = var.dynamodb_table_name },
  ], local.redis_password_environment)
}
