# --- Providers ---

provider "digitalocean" {
  token = var.do_token
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "kubernetes" {
  host                   = module.doks.endpoint
  cluster_ca_certificate = module.doks.cluster_ca_certificate

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "doctl"
    args = [
      "kubernetes",
      "cluster",
      "kubeconfig",
      "exec-credential",
      "--version=v1beta1",
      "--context=do",
      module.doks.cluster_id,
    ]
  }
}

# --- Modules ---

module "doks" {
  source = "../modules/doks"

  cluster_name = var.cluster_name
  region       = var.region
  k8s_version  = var.k8s_version
  node_size    = var.node_size
  min_nodes    = var.min_nodes
  max_nodes    = var.max_nodes
}

module "database" {
  source = "../modules/database"
  count  = var.enable_managed_database ? 1 : 0

  cluster_name        = var.cluster_name
  region              = var.region
  db_size             = var.db_size
  grafana_reader_user = var.managed_grafana_reader_user
  doks_cluster_id     = module.doks.cluster_id
}

module "storage" {
  source = "../modules/storage"
  count  = var.enable_observability_storage ? 1 : 0

  cloudflare_account_id = var.cloudflare_account_id
  cluster_name          = var.cluster_name
}

# --- Cloudflare API token secrets (for cert-manager and ExternalDNS) ---
# These must exist before Flux can reconcile cert-manager and external-dns.

resource "kubernetes_namespace" "cert_manager" {
  metadata {
    name = "cert-manager"
  }

  lifecycle {
    ignore_changes = [metadata[0].labels, metadata[0].annotations]
  }
}

resource "kubernetes_secret" "cloudflare_cert_manager" {
  metadata {
    name      = "cloudflare-api-token"
    namespace = "cert-manager"
  }

  data = {
    api-token = var.cloudflare_api_token
  }

  lifecycle {
    ignore_changes = [data, metadata[0].labels, metadata[0].annotations]
  }

  depends_on = [kubernetes_namespace.cert_manager]
}

resource "kubernetes_namespace" "external_dns" {
  metadata {
    name = "external-dns"
  }

  lifecycle {
    ignore_changes = [metadata[0].labels, metadata[0].annotations]
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

  lifecycle {
    ignore_changes = [data, metadata[0].labels, metadata[0].annotations]
  }

  depends_on = [kubernetes_namespace.external_dns]
}
