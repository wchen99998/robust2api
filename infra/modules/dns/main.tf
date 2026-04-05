resource "cloudflare_record" "app" {
  zone_id = var.cloudflare_zone_id
  name    = var.record_name
  content = var.record_value
  type    = "A"
  proxied = var.proxied
  ttl     = var.proxied ? 1 : 300 # Auto TTL when proxied
}
