variable "ingress_nginx_version" {
  description = "ingress-nginx Helm chart version"
  type        = string
  default     = "4.12.1"
}

variable "cert_manager_version" {
  description = "cert-manager Helm chart version"
  type        = string
  default     = "1.17.1"
}

variable "letsencrypt_email" {
  description = "Email for Let's Encrypt certificate notifications"
  type        = string
}

variable "app_namespace" {
  description = "Namespace to create for the application"
  type        = string
  default     = "sub2api"
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token (zone:read + dns:edit) for ExternalDNS and cert-manager DNS-01"
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID to restrict ExternalDNS record management"
  type        = string
}

variable "domain_suffix" {
  description = "Domain suffix for convention-based DNS (<service>-<namespace>.<suffix>)"
  type        = string
}

variable "cluster_name" {
  description = "Cluster name used as ExternalDNS TXT owner ID to prevent cross-cluster conflicts"
  type        = string
}

variable "cloudflare_proxied_default" {
  description = "Default Cloudflare proxy mode for ExternalDNS-managed records"
  type        = bool
  default     = true
}

variable "external_dns_version" {
  description = "external-dns Helm chart version"
  type        = string
  default     = "1.16.1"
}
