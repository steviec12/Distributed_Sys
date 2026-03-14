resource "aws_ecr_repository" "api" {
  name                 = "${var.project_name}-api"
  force_delete         = true
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-api"
  })
}

resource "aws_ecr_repository" "processor" {
  name                 = "${var.project_name}-processor"
  force_delete         = true
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(local.resource_tags, {
    Name = "${var.project_name}-processor"
  })
}
