data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_subnet" "default" {
  for_each = toset(data.aws_subnets.default.ids)
  id       = each.value
}

data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

locals {
  preferred_azs = ["us-west-2a", "us-west-2b", "us-west-2c"]

  candidate_default_subnet_ids = [
    for subnet_id, subnet in data.aws_subnet.default : subnet_id
    if contains(local.preferred_azs, subnet.availability_zone)
  ]

  selected_subnet_id = var.subnet_id != "" ? var.subnet_id : (
    length(local.candidate_default_subnet_ids) > 0
    ? local.candidate_default_subnet_ids[0]
    : data.aws_subnets.default.ids[0]
  )
  effective_app_access_cidrs = length(var.app_access_cidrs) > 0 ? var.app_access_cidrs : [var.ssh_cidr]

  db_nodes = {
    node1 = {
      node_id = "node-1"
      role    = "leader"
    }
    node2 = {
      node_id = "node-2"
      role    = "follower"
    }
    node3 = {
      node_id = "node-3"
      role    = "follower"
    }
    node4 = {
      node_id = "node-4"
      role    = "follower"
    }
    node5 = {
      node_id = "node-5"
      role    = "follower"
    }
  }

  common_tags = {
    Project   = var.project_name
    ManagedBy = "Terraform"
    Homework  = "HW10"
  }
}

resource "aws_security_group" "hw10" {
  name        = "${var.project_name}-sg"
  description = "Allow SSH and HW10 node traffic."
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_cidr]
  }

  ingress {
    description = "HW10 app from approved CIDRs"
    from_port   = var.app_port
    to_port     = var.app_port
    protocol    = "tcp"
    cidr_blocks = local.effective_app_access_cidrs
  }

  ingress {
    description = "Replica traffic within the HW10 security group"
    from_port   = var.app_port
    to_port     = var.app_port
    protocol    = "tcp"
    self        = true
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${var.project_name}-sg"
  })
}

resource "aws_instance" "db" {
  for_each = local.db_nodes

  ami                         = data.aws_ami.al2023.id
  instance_type               = var.instance_type
  subnet_id                   = local.selected_subnet_id
  vpc_security_group_ids      = [aws_security_group.hw10.id]
  key_name                    = var.ssh_key_name
  iam_instance_profile        = var.iam_instance_profile_name
  associate_public_ip_address = true

  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    node_name = each.key
    app_port  = var.app_port
  })

  tags = merge(local.common_tags, {
    Name   = "${var.project_name}-${each.key}"
    NodeID = each.value.node_id
    Role   = each.value.role
  })
}

resource "aws_instance" "tester" {
  count = var.create_tester_instance ? 1 : 0

  ami                         = data.aws_ami.al2023.id
  instance_type               = var.tester_instance_type
  subnet_id                   = local.selected_subnet_id
  vpc_security_group_ids      = [aws_security_group.hw10.id]
  key_name                    = var.ssh_key_name
  iam_instance_profile        = var.iam_instance_profile_name
  associate_public_ip_address = true

  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    node_name = "tester"
    app_port  = var.app_port
  })

  tags = merge(local.common_tags, {
    Name = "${var.project_name}-tester"
    Role = "tester"
  })
}
