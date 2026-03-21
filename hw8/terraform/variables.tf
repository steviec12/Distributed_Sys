variable "project_name" {
  description = "Name prefix for HW8 resources."
  type        = string
  default     = "hw8-shopping-cart"
}

variable "aws_region" {
  description = "AWS region for HW8 deployment."
  type        = string
  default     = "us-west-2"
}

variable "vpc_cidr" {
  description = "CIDR block for the HW8 VPC."
  type        = string
  default     = "10.8.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "CIDR blocks for the public subnets used by the ALB and NAT gateway."
  type        = list(string)
  default     = ["10.8.1.0/24", "10.8.2.0/24"]
}

variable "private_subnet_cidrs" {
  description = "CIDR blocks for the private subnets used by ECS tasks and RDS."
  type        = list(string)
  default     = ["10.8.10.0/24", "10.8.11.0/24"]
}

variable "container_port" {
  description = "Port exposed by the HW8 API container."
  type        = number
  default     = 8080
}

variable "container_image" {
  description = "Container image URI for the HW8 API."
  type        = string
}

variable "data_backend" {
  description = "Storage backend for the HW8 API."
  type        = string
  default     = "mysql"

  validation {
    condition     = contains(["mysql", "dynamodb"], var.data_backend)
    error_message = "data_backend must be either mysql or dynamodb."
  }
}

variable "task_cpu" {
  description = "CPU units for the ECS task."
  type        = number
  default     = 256
}

variable "task_memory" {
  description = "Memory in MiB for the ECS task."
  type        = number
  default     = 512
}

variable "api_desired_count" {
  description = "Desired number of ECS tasks for the API service."
  type        = number
  default     = 1
}

variable "enable_ecs_service" {
  description = "Create the ECS service once the image is ready."
  type        = bool
  default     = true
}

variable "ecs_task_role_name" {
  description = "Existing IAM role name used by Learner Lab ECS tasks."
  type        = string
  default     = "LabRole"
}

variable "log_retention_days" {
  description = "CloudWatch log retention for the API."
  type        = number
  default     = 7
}

variable "db_name" {
  description = "MySQL database name."
  type        = string
  default     = "hw8"
}

variable "db_username" {
  description = "MySQL master username."
  type        = string
  default     = "hw8user"
}

variable "db_password" {
  description = "MySQL master password."
  type        = string
  sensitive   = true
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t3.micro"
}

variable "db_allocated_storage" {
  description = "Allocated storage in GiB for RDS."
  type        = number
  default     = 20
}

variable "db_engine_version" {
  description = "MySQL engine version for RDS."
  type        = string
  default     = "8.0"
}

variable "dynamodb_table_name" {
  description = "DynamoDB table name for shopping carts."
  type        = string
  default     = "hw8-shopping-carts-dynamodb"
}
