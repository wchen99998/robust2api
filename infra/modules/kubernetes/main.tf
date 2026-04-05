resource "kubernetes_namespace" "app" {
  metadata {
    name = var.app_namespace
  }
}

# --- ingress-nginx ---

resource "helm_release" "ingress_nginx" {
  name             = "ingress-nginx"
  repository       = "https://kubernetes.github.io/ingress-nginx"
  chart            = "ingress-nginx"
  version          = var.ingress_nginx_version
  namespace        = "ingress-nginx"
  create_namespace = true
  wait             = true
  timeout          = 600

  set {
    name  = "controller.service.externalTrafficPolicy"
    value = "Local"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-name"
    value = "sub2api-lb"
  }

  set {
    name  = "controller.service.annotations.service\\.beta\\.kubernetes\\.io/do-loadbalancer-tls-passthrough"
    value = "true"
  }
}

# --- cert-manager ---

resource "helm_release" "cert_manager" {
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  version          = var.cert_manager_version
  namespace        = "cert-manager"
  create_namespace = true
  wait             = true
  timeout          = 600

  set {
    name  = "crds.enabled"
    value = "true"
  }
}

resource "kubernetes_manifest" "letsencrypt_issuer" {
  manifest = {
    apiVersion = "cert-manager.io/v1"
    kind       = "ClusterIssuer"
    metadata = {
      name = "letsencrypt-prod"
    }
    spec = {
      acme = {
        server = "https://acme-v02.api.letsencrypt.org/directory"
        email  = var.letsencrypt_email
        privateKeySecretRef = {
          name = "letsencrypt-prod"
        }
        solvers = [{
          dns01 = {
            cloudflare = {
              apiTokenSecretRef = {
                name = "cloudflare-api-token"
                key  = "api-token"
              }
            }
          }
        }]
      }
    }
  }

  depends_on = [helm_release.cert_manager, kubernetes_secret.cloudflare_cert_manager]
}

# --- Cloudflare API token secrets (for ExternalDNS and cert-manager DNS-01) ---

resource "kubernetes_secret" "cloudflare_cert_manager" {
  metadata {
    name      = "cloudflare-api-token"
    namespace = "cert-manager"
  }

  data = {
    api-token = var.cloudflare_api_token
  }

  depends_on = [helm_release.cert_manager]
}

# --- ExternalDNS ---

resource "kubernetes_namespace" "external_dns" {
  metadata {
    name = "external-dns"
  }
}

resource "kubernetes_secret" "cloudflare_external_dns" {
  metadata {
    name      = "cloudflare-api-token"
    namespace = "external-dns"
  }

  data = {
    api-token = var.cloudflare_api_token
  }

  depends_on = [kubernetes_namespace.external_dns]
}

resource "helm_release" "external_dns" {
  name             = "external-dns"
  repository       = "https://kubernetes-sigs.github.io/external-dns"
  chart            = "external-dns"
  version          = var.external_dns_version
  namespace        = "external-dns"
  create_namespace = false
  wait             = true
  timeout          = 300

  set {
    name  = "provider.name"
    value = "cloudflare"
  }

  set {
    name  = "env[0].name"
    value = "CF_API_TOKEN"
  }

  set {
    name  = "env[0].valueFrom.secretKeyRef.name"
    value = "cloudflare-api-token"
  }

  set {
    name  = "env[0].valueFrom.secretKeyRef.key"
    value = "api-token"
  }

  set {
    name  = "policy"
    value = "sync"
  }

  set {
    name  = "txtOwnerId"
    value = var.cluster_name
  }

  set {
    name  = "domainFilters[0]"
    value = var.domain_suffix
  }

  set {
    name  = "sources[0]"
    value = "ingress"
  }

  dynamic "set" {
    for_each = var.cloudflare_proxied_default ? [1] : []
    content {
      name  = "extraArgs[0]"
      value = "--cloudflare-proxied"
    }
  }

  depends_on = [kubernetes_secret.cloudflare_external_dns]
}

# --- Load balancer IP lookup ---

data "kubernetes_service" "ingress_nginx" {
  metadata {
    name      = "ingress-nginx-controller"
    namespace = "ingress-nginx"
  }

  depends_on = [helm_release.ingress_nginx]
}
