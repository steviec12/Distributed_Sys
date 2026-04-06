output "db_public_ips" {
  description = "Public IPs for the five database nodes."
  value = {
    for name, instance in aws_instance.db : name => instance.public_ip
  }
}

output "db_private_ips" {
  description = "Private IPs for the five database nodes. Use these for inter-node traffic."
  value = {
    for name, instance in aws_instance.db : name => instance.private_ip
  }
}

output "tester_public_ip" {
  description = "Public IP for the optional tester instance."
  value       = try(aws_instance.tester[0].public_ip, null)
}

output "ssh_commands" {
  description = "Convenience SSH commands for the provisioned instances."
  value = merge(
    {
      for name, instance in aws_instance.db :
      name => "ssh -i <PATH_TO_YOUR_PEM> ec2-user@${instance.public_ip}"
    },
    var.create_tester_instance ? {
      tester = "ssh -i <PATH_TO_YOUR_PEM> ec2-user@${aws_instance.tester[0].public_ip}"
    } : {}
  )
}

output "cloud_targets_env" {
  description = "Public URLs ready to paste into hw10/cloud_targets.env."
  value       = <<-EOT
LF_NODE1=http://${aws_instance.db["node1"].public_ip}:8080
LF_NODE2=http://${aws_instance.db["node2"].public_ip}:8080
LF_NODE3=http://${aws_instance.db["node3"].public_ip}:8080
LF_NODE4=http://${aws_instance.db["node4"].public_ip}:8080
LF_NODE5=http://${aws_instance.db["node5"].public_ip}:8080
LF_LEADER=http://${aws_instance.db["node1"].public_ip}:8080

LL_NODE1=http://${aws_instance.db["node1"].public_ip}:8080
LL_NODE2=http://${aws_instance.db["node2"].public_ip}:8080
LL_NODE3=http://${aws_instance.db["node3"].public_ip}:8080
LL_NODE4=http://${aws_instance.db["node4"].public_ip}:8080
LL_NODE5=http://${aws_instance.db["node5"].public_ip}:8080
EOT
}

output "private_node_urls" {
  description = "Private URLs to use in node environment variables for replica communication."
  value = {
    for name, instance in aws_instance.db : name => "http://${instance.private_ip}:8080"
  }
}
