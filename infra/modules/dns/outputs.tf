output "fqdn" {
  description = "DNS record name as created"
  value       = cloudflare_dns_record.app.name
}

output "record_id" {
  description = "Cloudflare DNS record ID"
  value       = cloudflare_dns_record.app.id
}
