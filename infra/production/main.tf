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

provider "helm" {
  kubernetes {
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

module "kubernetes" {
  source = "../modules/kubernetes"

  letsencrypt_email          = var.letsencrypt_email
  cloudflare_api_token       = var.cloudflare_api_token
  cloudflare_zone_id         = var.cloudflare_zone_id
  domain_suffix              = var.domain_suffix
  cluster_name               = var.cluster_name
  cloudflare_proxied_default = var.cloudflare_proxied

  depends_on = [module.doks]
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

check "monitoring_requires_observability_storage" {
  assert {
    condition     = !var.enable_monitoring || var.enable_observability_storage
    error_message = "enable_observability_storage must be true when enable_monitoring is enabled (R2 buckets required for Tempo/Loki)."
  }
}

check "monitoring_requires_r2_credentials" {
  assert {
    condition     = !var.enable_monitoring || (var.r2_access_key != "" && var.r2_secret_key != "")
    error_message = "r2_access_key and r2_secret_key must be set when enable_monitoring is enabled."
  }
}

locals {
  effective_grafana_admin_password = var.existing_grafana_admin_password != "" ? var.existing_grafana_admin_password : random_password.grafana_admin_password.result
  effective_grafana_db_host        = var.enable_managed_database ? module.database[0].host : var.grafana_db_host
  effective_grafana_db_port        = var.enable_managed_database ? module.database[0].port : var.grafana_db_port
  effective_grafana_db_name        = var.enable_managed_database ? module.database[0].database : var.grafana_db_name
  effective_grafana_db_user        = var.enable_managed_database ? module.database[0].grafana_reader_user : var.grafana_db_user
  effective_grafana_db_password    = var.enable_managed_database ? module.database[0].grafana_reader_password : var.grafana_db_password
  effective_grafana_db_sslmode     = var.enable_managed_database ? module.database[0].sslmode : var.grafana_db_sslmode
}

check "monitoring_requires_grafana_db_credentials" {
  assert {
    condition = !var.enable_monitoring || (
      local.effective_grafana_db_host != "" &&
      local.effective_grafana_db_name != "" &&
      local.effective_grafana_db_user != "" &&
      local.effective_grafana_db_password != ""
    )
    error_message = "Grafana PostgreSQL datasource credentials must be configured when enable_monitoring=true."
  }
}

# --- Auto-generated secrets ---

resource "random_password" "grafana_admin_password" {
  length  = 16
  special = true
}

# --- Monitoring (optional) ---

module "monitoring" {
  source = "../modules/monitoring"
  count  = var.enable_monitoring ? 1 : 0

  chart_path      = "${path.module}/../../deploy/helm/monitoring"
  domain_suffix   = var.domain_suffix
  hostname_prefix = var.grafana_hostname_prefix

  grafana_admin_password = local.effective_grafana_admin_password
  grafana_db_host        = local.effective_grafana_db_host
  grafana_db_port        = local.effective_grafana_db_port
  grafana_db_name        = local.effective_grafana_db_name
  grafana_db_user        = local.effective_grafana_db_user
  grafana_db_password    = local.effective_grafana_db_password
  grafana_db_sslmode     = local.effective_grafana_db_sslmode

  # R2 storage (from storage module)
  r2_endpoint   = var.enable_observability_storage ? module.storage[0].s3_endpoint : ""
  r2_access_key = var.r2_access_key
  r2_secret_key = var.r2_secret_key
  tempo_bucket  = var.enable_observability_storage ? module.storage[0].tempo_bucket : ""
  loki_bucket   = var.enable_observability_storage ? module.storage[0].loki_bucket : ""

  depends_on = [module.kubernetes]
}
