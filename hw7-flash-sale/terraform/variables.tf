variable "aws_region" {
  description = "AWS region for Homework 7 resources."
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Prefix used for Terraform-managed resource names."
  type        = string
  default     = "hw7-flash-sale"
}

variable "sns_topic_name" {
  description = "SNS topic used for order-created events."
  type        = string
  default     = "order-processing-events"
}

variable "sqs_queue_name" {
  description = "SQS queue consumed by the order processor."
  type        = string
  default     = "order-processing-queue"
}

variable "queue_visibility_timeout_seconds" {
  description = "How long a claimed message stays invisible to other consumers."
  type        = number
  default     = 30
}

variable "queue_message_retention_seconds" {
  description = "How long SQS keeps unprocessed messages."
  type        = number
  default     = 345600
}

variable "queue_receive_wait_time_seconds" {
  description = "Long polling wait time for ReceiveMessage."
  type        = number
  default     = 20
}

variable "attach_policies_to_lab_role" {
  description = "Attach the generated publish/consume policies to the existing Learner Lab role."
  type        = bool
  default     = false
}

variable "lab_role_name" {
  description = "Existing IAM role name to attach policies to when using Learner Lab."
  type        = string
  default     = "LabRole"
}

variable "common_tags" {
  description = "Tags applied to the messaging resources."
  type        = map(string)
  default = {
    Assignment = "HW7"
    Project    = "flash-sale"
  }
}
