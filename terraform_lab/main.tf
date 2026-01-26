provider "aws" {
  region = "us-west-2"
}

# --- VARIABLES ---
variable "ssh_key_name" {
  description = "The name of the existing AWS key pair"
  type        = string
}

variable "ssh_cidr" {
  description = "CIDR block for SSH access (your IP)"
  type        = string
}

# --- SECURITY GROUP ---
resource "aws_security_group" "ssh" {
  name        = "allow_ssh_http"
  description = "Allow SSH and HTTP"

  # SSH Rule
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_cidr]
  }

  # HTTP Rule (Port 8080)
  ingress {
    description = "HTTP"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Outbound Rule
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

# --- EC2 INSTANCE ---
data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64-ebs"]
  }
}

resource "aws_instance" "demo-instance" {
  count = 2 
  ami                    = data.aws_ami.al2023.id
  instance_type          = "t2.micro"
  iam_instance_profile   = "LabInstanceProfile"
  vpc_security_group_ids = [aws_security_group.ssh.id]
  key_name               = var.ssh_key_name

  tags = {
    Name = "terraform-created-instance-:)"
  }
}

# --- OUTPUTS ---
output "ec2_public_dns_list" {
  value = aws_instance.demo-instance[*].public_dns
}