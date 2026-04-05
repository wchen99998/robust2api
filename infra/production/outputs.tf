# --- Cluster ---

output "cluster_endpoint" {
  description = "Kubernetes API endpoint"
  value       = module.doks.endpoint
}

output "cluster_name" {
  description = "DOKS cluster name"
  value       = module.doks.name
}

output "kubeconfig_command" {
  description = "Command to configure kubectl"
  value       = "doctl kubernetes cluster kubeconfig save ${module.doks.name}"
}

# --- Networking ---

output "load_balancer_ip" {
  description = "Ingress load balancer IP"
  value       = module.kubernetes.load_balancer_ip
}

output "domain_suffix" {
  description = "Domain suffix for services (<service>-<namespace>.<suffix>)"
  value       = var.domain_suffix
}

# --- Database (conditional) ---

output "database_host" {
  description = "Managed PostgreSQL host (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].host : ""
}

output "database_port" {
  description = "Managed PostgreSQL port (empty if disabled)"
  value       = var.enable_managed_database ? tostring(module.database[0].port) : ""
}

output "database_user" {
  description = "Managed PostgreSQL user (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].user : ""
}

output "database_password" {
  description = "Managed PostgreSQL password (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].password : ""
  sensitive   = true
}

output "database_name" {
  description = "Managed PostgreSQL database name (empty if disabled)"
  value       = var.enable_managed_database ? module.database[0].database : ""
}
