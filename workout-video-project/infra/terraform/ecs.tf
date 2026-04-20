resource "aws_ecs_cluster" "main" {
  name = "${var.project_name}-cluster"

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-cluster"
  })
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.project_name}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = tostring(var.api_task_cpu)
  memory                   = tostring(var.api_task_memory)
  execution_role_arn       = local.learner_lab_role_arn
  task_role_arn            = local.learner_lab_role_arn

  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name      = local.api_container_name
      image     = var.api_image_uri
      essential = true
      portMappings = [
        {
          containerPort = var.api_container_port
          protocol      = "tcp"
        }
      ]
      environment = local.api_environment
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.api.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.project_name}-worker"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = tostring(var.worker_task_cpu)
  memory                   = tostring(var.worker_task_memory)
  execution_role_arn       = local.learner_lab_role_arn
  task_role_arn            = local.learner_lab_role_arn

  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name        = local.worker_container_name
      image       = var.worker_image_uri
      essential   = true
      stopTimeout = var.worker_stop_timeout_seconds
      environment = local.worker_environment
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.worker.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "reaper" {
  family                   = "${var.project_name}-reaper"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = tostring(var.reaper_task_cpu)
  memory                   = tostring(var.reaper_task_memory)
  execution_role_arn       = local.learner_lab_role_arn
  task_role_arn            = local.learner_lab_role_arn

  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name        = local.reaper_container_name
      image       = var.reaper_image_uri
      essential   = true
      environment = local.reaper_environment
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.reaper.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "api" {
  name                               = "${var.project_name}-api"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.api.arn
  desired_count                      = var.api_desired_count
  launch_type                        = "FARGATE"
  platform_version                   = var.service_platform_version
  enable_execute_command             = var.enable_execute_command
  health_check_grace_period_seconds  = 60
  deployment_maximum_percent         = 200
  deployment_minimum_healthy_percent = 100

  network_configuration {
    subnets          = local.effective_subnet_ids
    security_groups  = [aws_security_group.api_service.id]
    assign_public_ip = var.assign_public_ip
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = local.api_container_name
    container_port   = var.api_container_port
  }

  depends_on = [aws_lb_listener.http]

  lifecycle {
    precondition {
      condition     = length(local.effective_subnet_ids) >= 2
      error_message = "Provide at least two subnet IDs, or allow Terraform to discover at least two subnets in the selected VPC."
    }
  }
}

resource "aws_ecs_service" "worker" {
  name                               = "${var.project_name}-worker"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.worker.arn
  desired_count                      = var.worker_desired_count
  launch_type                        = "FARGATE"
  platform_version                   = var.service_platform_version
  enable_execute_command             = var.enable_execute_command
  deployment_maximum_percent         = 200
  deployment_minimum_healthy_percent = 100

  network_configuration {
    subnets          = local.effective_subnet_ids
    security_groups  = [aws_security_group.worker_service.id]
    assign_public_ip = var.assign_public_ip
  }

  lifecycle {
    precondition {
      condition     = length(local.effective_subnet_ids) >= 2
      error_message = "Provide at least two subnet IDs, or allow Terraform to discover at least two subnets in the selected VPC."
    }
  }
}

resource "aws_ecs_service" "reaper" {
  name                               = "${var.project_name}-reaper"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.reaper.arn
  desired_count                      = var.reaper_desired_count
  launch_type                        = "FARGATE"
  platform_version                   = var.service_platform_version
  enable_execute_command             = var.enable_execute_command
  deployment_maximum_percent         = 100
  deployment_minimum_healthy_percent = 0

  network_configuration {
    subnets          = local.effective_subnet_ids
    security_groups  = [aws_security_group.reaper_service.id]
    assign_public_ip = var.assign_public_ip
  }

  lifecycle {
    precondition {
      condition     = length(local.effective_subnet_ids) >= 2
      error_message = "Provide at least two subnet IDs, or allow Terraform to discover at least two subnets in the selected VPC."
    }
  }
}
