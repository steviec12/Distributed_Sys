variable "vpc_cidr" {
  description = "CIDR block for the Homework 7 VPC."
  type        = string
  default     = "10.0.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for the public ALB subnets."
  type        = list(string)
  default     = ["10.0.1.0/24", "10.0.2.0/24"]
}

variable "private_subnet_cidrs" {
  description = "CIDR blocks for the private ECS subnets."
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "container_port" {
  description = "Port exposed by the API container."
  type        = number
  default     = 8080
}

variable "task_cpu" {
  description = "CPU units for each ECS task."
  type        = number
  default     = 256
}

variable "task_memory" {
  description = "Memory in MiB for each ECS task."
  type        = number
  default     = 512
}

variable "payment_delay_seconds" {
  description = "Shared payment delay used by API sync and processor workers."
  type        = number
  default     = 3
}

variable "sync_payment_slots" {
  description = "Number of concurrent sync payment slots exposed by the API."
  type        = number
  default     = 1
}

variable "processor_worker_count" {
  description = "Number of worker goroutines used by the processor task."
  type        = number
  default     = 1
}

variable "api_image_tag" {
  description = "Container image tag used by the API task definition."
  type        = string
  default     = "latest"
}

variable "processor_image_tag" {
  description = "Container image tag used by the processor task definition."
  type        = string
  default     = "latest"
}

variable "enable_ecs_services" {
  description = "Create the ECS services and ALB target registration. Keep false until images are pushed."
  type        = bool
  default     = false
}

variable "enable_lambda_subscriber" {
  description = "Create the Part III Lambda subscriber and subscribe it to the SNS topic."
  type        = bool
  default     = false
}

variable "api_desired_count" {
  description = "Desired task count for the API ECS service."
  type        = number
  default     = 1
}

variable "processor_desired_count" {
  description = "Desired task count for the processor ECS service."
  type        = number
  default     = 1
}

variable "log_retention_days" {
  description = "CloudWatch log retention for ECS task logs."
  type        = number
  default     = 7
}

variable "ecs_task_role_name" {
  description = "Existing IAM role name used by Learner Lab ECS tasks."
  type        = string
  default     = "LabRole"
}
