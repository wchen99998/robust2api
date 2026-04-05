output "fqdn" {
  description = "Fully qualified domain name of the record"
  value       = cloudflare_record.app.hostname
}

output "record_id" {
  description = "Cloudflare DNS record ID"
  value       = cloudflare_record.app.id
}
