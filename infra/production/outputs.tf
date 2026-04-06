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

# --- Application ---

output "app_url" {
  description = "Sub2API application URL"
  value       = "https://sub2api-${module.kubernetes.app_namespace}.${var.domain_suffix}"
}

output "admin_password" {
  description = "Auto-generated admin password"
  value       = random_password.admin_password.result
  sensitive   = true
}

output "jwt_secret" {
  description = "Auto-generated JWT signing secret"
  value       = random_password.jwt_secret.result
  sensitive   = true
}

output "totp_encryption_key" {
  description = "Auto-generated TOTP encryption key (64 hex chars)"
  value       = random_id.totp_encryption_key.hex
  sensitive   = true
}

# --- Monitoring (conditional) ---

output "grafana_url" {
  description = "Grafana dashboard URL (empty if monitoring disabled)"
  value       = var.enable_monitoring ? "https://grafana-monitoring.${var.domain_suffix}" : ""
}

output "grafana_admin_password" {
  description = "Auto-generated Grafana admin password (empty if monitoring disabled)"
  value       = var.enable_monitoring ? random_password.grafana_admin_password.result : ""
  sensitive   = true
}
