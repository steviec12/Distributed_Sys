resource "aws_security_group" "alb" {
  name_prefix = "${var.project_name}-alb-"
  description = "Allow inbound HTTP traffic to the workout video API ALB."
  vpc_id      = local.effective_vpc_id

  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = var.alb_ingress_cidrs
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-alb-sg"
  })
}

resource "aws_security_group" "api_service" {
  name_prefix = "${var.project_name}-api-"
  description = "Allow ALB traffic to the workout video API tasks."
  vpc_id      = local.effective_vpc_id

  ingress {
    description     = "ALB to API"
    from_port       = var.api_container_port
    to_port         = var.api_container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-api-sg"
  })
}

resource "aws_security_group" "worker_service" {
  name_prefix = "${var.project_name}-worker-"
  description = "Allow outbound traffic from workout video worker tasks."
  vpc_id      = local.effective_vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-worker-sg"
  })
}

resource "aws_security_group" "reaper_service" {
  name_prefix = "${var.project_name}-reaper-"
  description = "Allow outbound traffic from workout video reaper tasks."
  vpc_id      = local.effective_vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-reaper-sg"
  })
}
