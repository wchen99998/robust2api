variable "cloudflare_zone_id" {
  description = "Cloudflare zone ID"
  type        = string
}

variable "record_name" {
  description = "DNS record name (e.g. 'api' or 'sub2api')"
  type        = string
}

variable "record_value" {
  description = "IP address to point the record to (load balancer IP)"
  type        = string
}

variable "proxied" {
  description = "Enable Cloudflare proxy (CDN/WAF)"
  type        = bool
  default     = true
}
