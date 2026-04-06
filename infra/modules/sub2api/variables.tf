# --- Chart ---

variable "chart_path" {
  description = "Local path to the sub2api Helm chart directory"
  type        = string
}

variable "namespace" {
  description = "Kubernetes namespace for the release"
  type        = string
}

variable "app_image_tag" {
  description = "Container image tag to deploy"
  type        = string
}

variable "domain_suffix" {
  description = "Domain suffix for ingress host convention (sub2api-<ns>.<suffix>)"
  type        = string
}

# --- Database (optional — empty strings = use in-cluster subchart) ---

variable "database_host" {
  description = "External database host (empty = use in-cluster PostgreSQL subchart)"
  type        = string
  default     = ""
}

variable "database_port" {
  description = "External database port"
  type        = number
  default     = 5432
}

variable "database_user" {
  description = "External database user"
  type        = string
  default     = "sub2api"
}

variable "database_password" {
  description = "External database password"
  type        = string
  default     = ""
  sensitive   = true
}

variable "database_name" {
  description = "External database name"
  type        = string
  default     = "sub2api"
}

# --- In-cluster service credentials ---

variable "in_cluster_postgresql_password" {
  description = "Password for the in-cluster PostgreSQL subchart"
  type        = string
  sensitive   = true
}

variable "in_cluster_redis_password" {
  description = "Password for the in-cluster Redis subchart"
  type        = string
  sensitive   = true
}

# --- Secrets ---

variable "jwt_secret" {
  description = "JWT signing secret"
  type        = string
  sensitive   = true
}

variable "totp_encryption_key" {
  description = "TOTP encryption key"
  type        = string
  sensitive   = true
}

variable "admin_email" {
  description = "Initial admin email"
  type        = string
  default     = "admin@sub2api.local"
}

variable "admin_password" {
  description = "Initial admin password"
  type        = string
  sensitive   = true
}
