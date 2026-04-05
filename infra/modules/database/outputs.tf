output "host" {
  description = "Database host"
  value       = digitalocean_database_cluster.postgres.host
}

output "port" {
  description = "Database port"
  value       = digitalocean_database_cluster.postgres.port
}

output "user" {
  description = "Database user"
  value       = digitalocean_database_user.app.name
}

output "password" {
  description = "Database password"
  value       = digitalocean_database_user.app.password
  sensitive   = true
}

output "database" {
  description = "Database name"
  value       = digitalocean_database_db.app.name
}

output "sslmode" {
  description = "SSL mode for database connections"
  value       = "require"
}

output "uri" {
  description = "Full connection URI"
  value       = digitalocean_database_cluster.postgres.uri
  sensitive   = true
}
