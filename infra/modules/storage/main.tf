# Cloudflare R2 buckets for observability backends (Tempo traces, Loki logs/chunks).
# R2 is S3-compatible with zero egress fees.

resource "cloudflare_r2_bucket" "tempo" {
  account_id = var.cloudflare_account_id
  name       = "${var.cluster_name}-tempo"
  location   = var.r2_location
}

resource "cloudflare_r2_bucket" "loki" {
  account_id = var.cloudflare_account_id
  name       = "${var.cluster_name}-loki"
  location   = var.r2_location
}
