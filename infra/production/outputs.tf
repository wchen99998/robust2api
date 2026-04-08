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

output "grafana_reader_user" {
  description = "Grafana read-only DB user from managed PostgreSQL (empty if managed DB disabled)"
  value       = var.enable_managed_database ? module.database[0].grafana_reader_user : ""
}

output "grafana_reader_password" {
  description = "Grafana read-only DB password from managed PostgreSQL (empty if managed DB disabled)"
  value       = var.enable_managed_database ? module.database[0].grafana_reader_password : ""
  sensitive   = true
}

# --- Observability storage (conditional) ---

output "r2_tempo_bucket" {
  description = "Tempo R2 bucket name (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].tempo_bucket : ""
}

output "r2_loki_bucket" {
  description = "Loki R2 bucket name (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].loki_bucket : ""
}

output "r2_s3_endpoint" {
  description = "R2 S3-compatible endpoint (empty if disabled)"
  value       = var.enable_observability_storage ? module.storage[0].s3_endpoint : ""
}
