output "tempo_bucket" {
  description = "Tempo R2 bucket name"
  value       = cloudflare_r2_bucket.tempo.name
}

output "loki_bucket" {
  description = "Loki R2 bucket name"
  value       = cloudflare_r2_bucket.loki.name
}

output "s3_endpoint" {
  description = "R2 S3-compatible endpoint"
  value       = "https://${var.cloudflare_account_id}.r2.cloudflarestorage.com"
}
