variable "aws_region" {
  description = "AWS region for the ECS/Fargate deployment."
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Prefix used for Terraform-managed workout video resources."
  type        = string
  default     = "workout-video"
}

variable "common_tags" {
  description = "Additional tags applied to Terraform-managed resources."
  type        = map(string)
  default = {
    Assignment = "FinalProject"
  }
}

variable "vpc_id" {
  description = "Existing VPC ID. Leave empty to use the default VPC, which is usually the easiest Learner Lab path."
  type        = string
  default     = ""
}

variable "subnet_ids" {
  description = "Existing subnet IDs for the ALB and ECS services. Leave empty to auto-select two subnets from the chosen VPC."
  type        = list(string)
  default     = []

  validation {
    condition     = length(var.subnet_ids) == 0 || length(var.subnet_ids) >= 2
    error_message = "subnet_ids must be empty or contain at least two subnet IDs."
  }
}

variable "alb_ingress_cidrs" {
  description = "CIDR blocks allowed to reach the public API ALB."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "api_image_uri" {
  description = "Container image URI for the API service."
  type        = string
}

variable "worker_image_uri" {
  description = "Container image URI for the worker service."
  type        = string
}

variable "reaper_image_uri" {
  description = "Container image URI for the reaper service."
  type        = string
}

variable "redis_endpoint" {
  description = "Redis endpoint in host:port form."
  type        = string
}

variable "redis_password" {
  description = "Optional Redis AUTH password. Leave empty when the Redis deployment does not require authentication."
  type        = string
  default     = ""
  sensitive   = true
}

variable "dynamodb_table_name" {
  description = "Existing DynamoDB table name used for durable job state."
  type        = string
}

variable "s3_bucket_name" {
  description = "Existing S3 bucket name used for uploaded video objects."
  type        = string
}

variable "api_container_port" {
  description = "Container port exposed by the API service."
  type        = number
  default     = 8080
}

variable "service_platform_version" {
  description = "Fargate platform version for all ECS services."
  type        = string
  default     = "LATEST"
}

variable "enable_execute_command" {
  description = "Whether ECS Exec should be enabled for the services."
  type        = bool
  default     = true
}

variable "assign_public_ip" {
  description = "Whether the ECS services should receive public IPs. True is the pragmatic Learner Lab default."
  type        = bool
  default     = true
}

variable "api_desired_count" {
  description = "Desired task count for the API service."
  type        = number
  default     = 1
}

variable "worker_desired_count" {
  description = "Desired task count for the worker service."
  type        = number
  default     = 1
}

variable "reaper_desired_count" {
  description = "Desired task count for the reaper service."
  type        = number
  default     = 1
}

variable "api_task_cpu" {
  description = "CPU units for the API task definition."
  type        = number
  default     = 512
}

variable "api_task_memory" {
  description = "Memory in MiB for the API task definition."
  type        = number
  default     = 1024
}

variable "worker_task_cpu" {
  description = "CPU units for the worker task definition."
  type        = number
  default     = 1024
}

variable "worker_task_memory" {
  description = "Memory in MiB for the worker task definition."
  type        = number
  default     = 2048
}

variable "worker_stop_timeout_seconds" {
  description = "How long ECS should wait after SIGTERM before force-killing the worker container."
  type        = number
  default     = 120
}

variable "reaper_task_cpu" {
  description = "CPU units for the reaper task definition."
  type        = number
  default     = 256
}

variable "reaper_task_memory" {
  description = "Memory in MiB for the reaper task definition."
  type        = number
  default     = 512
}

variable "log_retention_days" {
  description = "CloudWatch log retention for ECS services."
  type        = number
  default     = 7
}

variable "api_health_check_path" {
  description = "ALB health check path for the API target group."
  type        = string
  default     = "/health"
}

variable "api_health_check_matcher" {
  description = "Expected HTTP matcher for API ALB health checks."
  type        = string
  default     = "200"
}

variable "redis_db" {
  description = "Redis DB number shared by the services."
  type        = number
  default     = 0
}

variable "multipart_part_size_bytes" {
  description = "Multipart upload part size used by the API when creating presigned upload sessions."
  type        = number
  default     = 5242880
}

variable "s3_presign_expiration_seconds" {
  description = "Presigned URL lifetime for multipart uploads."
  type        = number
  default     = 3600
}

variable "heartbeat_interval_seconds" {
  description = "Worker heartbeat interval in seconds."
  type        = number
  default     = 2
}

variable "reaper_interval_seconds" {
  description = "Reaper polling interval in seconds."
  type        = number
  default     = 3
}

variable "stale_timeout_seconds" {
  description = "How old a heartbeat can be before the reaper treats the job as stale."
  type        = number
  default     = 12
}

variable "max_retries" {
  description = "Maximum retry count used by worker and reaper recovery logic."
  type        = number
  default     = 3
}
