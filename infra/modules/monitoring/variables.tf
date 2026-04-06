# --- Chart ---

variable "chart_path" {
  description = "Local path to the monitoring Helm chart directory"
  type        = string
}

variable "domain_suffix" {
  description = "Domain suffix for Grafana ingress (grafana-monitoring.<suffix>)"
  type        = string
}

# --- Grafana ---

variable "grafana_admin_password" {
  description = "Grafana admin password"
  type        = string
  sensitive   = true
}

# --- R2 Storage ---

variable "r2_endpoint" {
  description = "R2 S3-compatible endpoint URL"
  type        = string
}

variable "r2_access_key" {
  description = "R2 access key ID"
  type        = string
  sensitive   = true
}

variable "r2_secret_key" {
  description = "R2 secret access key"
  type        = string
  sensitive   = true
}

variable "tempo_bucket" {
  description = "R2 bucket name for Tempo traces"
  type        = string
}

variable "loki_bucket" {
  description = "R2 bucket name for Loki logs"
  type        = string
}
