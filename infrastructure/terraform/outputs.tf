output "control_plane_ipv4" {
  description = "Public IPv4 address of the InfraOS control plane reverse proxy."
  value       = module.control_plane.ipv4_address
}

output "control_plane_private_ip" {
  description = "Private VPC IPv4 address of the InfraOS control plane."
  value       = module.control_plane.private_ip
}

output "database_node_private_ip" {
  description = "Private VPC IPv4 address of the optional dedicated PostgreSQL node."
  value       = var.enable_dedicated_db_node ? module.database_node[0].private_ip : null
}

output "private_network_cidr" {
  description = "Private network CIDR block."
  value       = hcloud_network.private.ip_range
}
