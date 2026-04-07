# --- Chart ---

variable "chart_path" {
  description = "Local path to the monitoring Helm chart directory"
  type        = string
}

variable "domain_suffix" {
  description = "Domain suffix for Grafana ingress (grafana-monitoring.<suffix>)"
  type        = string
}

variable "hostname_prefix" {
  description = "Hostname prefix used for the Grafana URL (<prefix>.<suffix>)"
  type        = string
  default     = "grafana"
}

# --- Grafana ---

variable "grafana_admin_password" {
  description = "Grafana admin password"
  type        = string
  sensitive   = true
}

variable "grafana_db_host" {
  description = "PostgreSQL host for Grafana business analytics datasource"
  type        = string
}

variable "grafana_db_port" {
  description = "PostgreSQL port for Grafana business analytics datasource"
  type        = number
}

variable "grafana_db_name" {
  description = "PostgreSQL database name for Grafana business analytics datasource"
  type        = string
}

variable "grafana_db_user" {
  description = "PostgreSQL read-only user for Grafana business analytics datasource"
  type        = string
}

variable "grafana_db_password" {
  description = "PostgreSQL read-only password for Grafana business analytics datasource"
  type        = string
  sensitive   = true
}

variable "grafana_db_sslmode" {
  description = "PostgreSQL SSL mode for Grafana business analytics datasource"
  type        = string
  default     = "require"
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
