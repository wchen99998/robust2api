output "load_balancer_ip" {
  description = "External IP of the ingress-nginx load balancer"
  value       = data.kubernetes_service.ingress_nginx.status[0].load_balancer[0].ingress[0].ip
}

output "app_namespace" {
  description = "Name of the application namespace"
  value       = kubernetes_namespace.app.metadata[0].name
}

output "domain_suffix" {
  description = "Domain suffix for convention-based DNS"
  value       = var.domain_suffix
}
