output "cluster_id" {
  description = "ID of the DOKS cluster"
  value       = digitalocean_kubernetes_cluster.this.id
}

output "cluster_urn" {
  description = "URN of the DOKS cluster"
  value       = digitalocean_kubernetes_cluster.this.urn
}

output "endpoint" {
  description = "Kubernetes API endpoint"
  value       = digitalocean_kubernetes_cluster.this.endpoint
}

output "kubeconfig" {
  description = "Raw kubeconfig for the cluster"
  value       = digitalocean_kubernetes_cluster.this.kube_config[0]
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "CA certificate for the cluster"
  value       = base64decode(digitalocean_kubernetes_cluster.this.kube_config[0].cluster_ca_certificate)
  sensitive   = true
}

output "token" {
  description = "Authentication token for the cluster"
  value       = digitalocean_kubernetes_cluster.this.kube_config[0].token
  sensitive   = true
}

output "name" {
  description = "Name of the cluster"
  value       = digitalocean_kubernetes_cluster.this.name
}
