output "vpc_id" {
  description = "Homework 7 VPC ID."
  value       = aws_vpc.main.id
}

output "public_subnet_ids" {
  description = "Public subnet IDs used by the ALB."
  value       = aws_subnet.public[*].id
}

output "private_subnet_ids" {
  description = "Private subnet IDs used by the ECS tasks."
  value       = aws_subnet.private[*].id
}

output "alb_dns_name" {
  description = "Public DNS name of the API load balancer."
  value       = aws_lb.api.dns_name
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster."
  value       = aws_ecs_cluster.main.name
}

output "api_service_name" {
  description = "Name of the API ECS service when enabled."
  value       = length(aws_ecs_service.api) > 0 ? aws_ecs_service.api[0].name : null
}

output "processor_service_name" {
  description = "Name of the processor ECS service when enabled."
  value       = length(aws_ecs_service.processor) > 0 ? aws_ecs_service.processor[0].name : null
}

output "api_ecr_repository_url" {
  description = "ECR repository URL for the API image."
  value       = aws_ecr_repository.api.repository_url
}

output "processor_ecr_repository_url" {
  description = "ECR repository URL for the processor image."
  value       = aws_ecr_repository.processor.repository_url
}

output "api_image_uri" {
  description = "Full image URI expected by the API task definition."
  value       = local.api_image_uri
}

output "processor_image_uri" {
  description = "Full image URI expected by the processor task definition."
  value       = local.processor_image_uri
}

output "lambda_function_name" {
  description = "Name of the Part III Lambda subscriber when enabled."
  value       = length(aws_lambda_function.order_processor) > 0 ? aws_lambda_function.order_processor[0].function_name : null
}

output "lambda_log_group_name" {
  description = "CloudWatch log group for the Part III Lambda subscriber when enabled."
  value       = length(aws_cloudwatch_log_group.lambda_processor) > 0 ? aws_cloudwatch_log_group.lambda_processor[0].name : null
}
