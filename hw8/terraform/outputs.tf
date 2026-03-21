output "alb_dns_name" {
  description = "DNS name of the HW8 ALB."
  value       = aws_lb.api.dns_name
}

output "ecs_cluster_name" {
  description = "Name of the HW8 ECS cluster."
  value       = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  description = "Name of the HW8 ECS service."
  value       = length(aws_ecs_service.api) > 0 ? aws_ecs_service.api[0].name : null
}

output "rds_endpoint" {
  description = "RDS endpoint for the HW8 MySQL instance."
  value       = aws_db_instance.main.address
}

output "rds_port" {
  description = "RDS port for the HW8 MySQL instance."
  value       = aws_db_instance.main.port
}

output "dynamodb_table_name" {
  description = "DynamoDB shopping cart table name."
  value       = aws_dynamodb_table.shopping_carts.name
}
