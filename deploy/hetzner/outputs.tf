output "url" {
  description = "LibreDash HTTPS URL."
  value       = "https://${local.domain}"
}

output "server_ipv4" {
  description = "Reserved public IPv4."
  value       = hcloud_primary_ip.libredash.ip_address
}

output "ssh_command" {
  description = "SSH command for retrieving first-login credentials and inspecting logs."
  value       = "ssh${local.ssh_identity_arg} root@${hcloud_primary_ip.libredash.ip_address}"
}

output "initial_local_user_command" {
  description = "One-use command that retrieves and removes the initial local user's temporary password."
  value       = "ssh${local.ssh_identity_arg} root@${hcloud_primary_ip.libredash.ip_address} 'libredashctl first-login'"
}

output "operations_command" {
  description = "SSH prefix for status, logs, backup, restore, upgrade, and rollback operations."
  value       = "ssh${local.ssh_identity_arg} root@${hcloud_primary_ip.libredash.ip_address} libredashctl"
}
