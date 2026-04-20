output "cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.main.name
}

output "cluster_arn" {
  description = "ECS cluster ARN."
  value       = aws_ecs_cluster.main.arn
}

output "api_service_name" {
  description = "API ECS service name."
  value       = aws_ecs_service.api.name
}

output "worker_service_name" {
  description = "Worker ECS service name."
  value       = aws_ecs_service.worker.name
}

output "reaper_service_name" {
  description = "Reaper ECS service name."
  value       = aws_ecs_service.reaper.name
}

output "api_alb_dns_name" {
  description = "Public DNS name of the API ALB."
  value       = aws_lb.api.dns_name
}

output "api_target_group_arn" {
  description = "ARN of the API ALB target group."
  value       = aws_lb_target_group.api.arn
}

output "selected_vpc_id" {
  description = "VPC ID used by the deployment."
  value       = local.effective_vpc_id
}

output "selected_subnet_ids" {
  description = "Subnet IDs used by the ALB and ECS services."
  value       = local.effective_subnet_ids
}

output "security_group_ids" {
  description = "Security group IDs for ALB, API, worker, and reaper."
  value = {
    alb    = aws_security_group.alb.id
    api    = aws_security_group.api_service.id
    worker = aws_security_group.worker_service.id
    reaper = aws_security_group.reaper_service.id
  }
}
