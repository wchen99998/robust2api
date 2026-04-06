# --- Provider credentials ---

variable "do_token" {
  description = "DigitalOcean API token"
  type        = string
  sensitive   = true
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token with DNS edit permissions"
  type        = string
  sensitive   = true
}

# --- DOKS cluster ---

variable "region" {
  description = "DigitalOcean region"
  type        = string
  default     = "sgp1"
}

variable "cluster_name" {
  description = "DOKS cluster name"
  type        = string
  default     = "sub2api"
}

variable "k8s_version" {
  description = "Kubernetes version prefix"
  type        = string
  default     = "1.34"
}

variable "node_size" {
  description = "Droplet size for worker nodes"
  type        = string
  default     = "s-2vcpu-4gb"
}

variable "min_nodes" {
  description = "Autoscaler minimum nodes"
  type        = number
  default     = 1
}

variable "max_nodes" {
  description = "Autoscaler maximum nodes"
  type        = number
  default     = 3
}

# --- Kubernetes bootstrap ---

variable "letsencrypt_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
}

# --- DNS ---

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID for the domain"
  type        = string
}

variable "domain_suffix" {
  description = "Domain suffix for convention-based DNS (<service>-<namespace>.<suffix>)"
  type        = string
}

variable "cloudflare_proxied" {
  description = "Enable Cloudflare proxy (CDN/WAF)"
  type        = bool
  default     = true
}

# --- Database (optional) ---

variable "enable_managed_database" {
  description = "Create a DO Managed PostgreSQL instance"
  type        = bool
  default     = false
}

variable "db_size" {
  description = "Database droplet size (only used when enable_managed_database=true)"
  type        = string
  default     = "db-s-1vcpu-1gb"
}

# --- Observability storage (optional) ---

variable "enable_observability_storage" {
  description = "Create Cloudflare R2 buckets for Tempo and Loki"
  type        = bool
  default     = false
}

variable "cloudflare_account_id" {
  description = "Cloudflare account ID (required when enable_observability_storage=true)"
  type        = string
  default     = ""
}

# --- Sub2API application ---

variable "app_image_tag" {
  description = "Container image tag for the sub2api deployment"
  type        = string
}

variable "admin_email" {
  description = "Initial admin account email"
  type        = string
  default     = "admin@sub2api.local"
}

# --- Monitoring (optional) ---

variable "enable_monitoring" {
  description = "Deploy the LGTM monitoring stack via Helm"
  type        = bool
  default     = false
}

variable "r2_access_key" {
  description = "Cloudflare R2 access key for Tempo/Loki storage (required when enable_monitoring=true)"
  type        = string
  default     = ""
  sensitive   = true
}

variable "r2_secret_key" {
  description = "Cloudflare R2 secret key for Tempo/Loki storage (required when enable_monitoring=true)"
  type        = string
  default     = ""
  sensitive   = true
}
