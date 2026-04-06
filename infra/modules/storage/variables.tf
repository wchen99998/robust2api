variable "cloudflare_account_id" {
  description = "Cloudflare account ID"
  type        = string
}

variable "cluster_name" {
  description = "Cluster name used as prefix for bucket names"
  type        = string
}

variable "r2_location" {
  description = "R2 bucket location hint (best-effort). Available: apac, eeur, enam, weur, wnam, oc"
  type        = string
  default     = "apac"
}
