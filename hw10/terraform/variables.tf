variable "aws_region" {
  description = "AWS region for the HW10 EC2 instances."
  type        = string
  default     = "us-west-2"
}

variable "project_name" {
  description = "Tag prefix used for HW10 resources."
  type        = string
  default     = "hw10-replication"
}

variable "ssh_key_name" {
  description = "Name of the existing EC2 key pair to use for SSH."
  type        = string
}

variable "ssh_cidr" {
  description = "CIDR block allowed to SSH into the instances."
  type        = string
}

variable "app_access_cidrs" {
  description = "CIDR blocks allowed to access the app port from outside the security group."
  type        = list(string)
  default     = []
}

variable "instance_type" {
  description = "EC2 instance type for the database nodes."
  type        = string
  default     = "t2.micro"
}

variable "tester_instance_type" {
  description = "EC2 instance type for the optional test/load generator node."
  type        = string
  default     = "t2.micro"
}

variable "create_tester_instance" {
  description = "Whether to create an extra EC2 instance for cloud-side tests and load generation."
  type        = bool
  default     = true
}

variable "app_port" {
  description = "Port exposed by the HW10 node service."
  type        = number
  default     = 8080
}

variable "iam_instance_profile_name" {
  description = "Existing IAM instance profile name available in Learner Lab."
  type        = string
  default     = "LabInstanceProfile"
}

variable "subnet_id" {
  description = "Optional subnet ID. Leave empty to use the first default subnet in the default VPC."
  type        = string
  default     = ""
}
