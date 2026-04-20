resource "aws_lb" "api" {
  name               = substr("${var.project_name}-alb", 0, 32)
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = local.effective_subnet_ids

  lifecycle {
    precondition {
      condition     = length(local.effective_subnet_ids) >= 2
      error_message = "Provide at least two subnet IDs, or allow Terraform to discover at least two subnets in the selected VPC."
    }
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-alb"
  })
}

resource "aws_lb_target_group" "api" {
  name        = substr("${var.project_name}-api-tg", 0, 32)
  port        = var.api_container_port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = local.effective_vpc_id

  health_check {
    enabled             = true
    path                = var.api_health_check_path
    protocol            = "HTTP"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
    matcher             = var.api_health_check_matcher
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-api-tg"
  })
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.api.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }
}
