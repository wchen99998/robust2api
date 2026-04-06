# --- Providers ---

provider "digitalocean" {
  token = var.do_token
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

provider "kubernetes" {
  host                   = module.doks.endpoint
  token                  = module.doks.token
  cluster_ca_certificate = module.doks.cluster_ca_certificate
}

provider "helm" {
  kubernetes {
    host                   = module.doks.endpoint
    token                  = module.doks.token
    cluster_ca_certificate = module.doks.cluster_ca_certificate
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

  cluster_name    = var.cluster_name
  region          = var.region
  db_size         = var.db_size
  doks_cluster_id = module.doks.cluster_id
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

# --- Auto-generated secrets ---

resource "random_password" "jwt_secret" {
  length  = 32
  special = true
}

resource "random_id" "totp_encryption_key" {
  byte_length = 32
}

resource "random_password" "sub2api_postgresql_password" {
  length  = 32
  special = true
}

resource "random_password" "sub2api_redis_password" {
  length  = 32
  special = true
}

resource "random_password" "admin_password" {
  length  = 16
  special = true
}

resource "random_password" "grafana_admin_password" {
  length  = 16
  special = true
}

# --- Application ---

module "sub2api" {
  source = "../modules/sub2api"

  chart_path    = "${path.module}/../../deploy/helm/sub2api"
  namespace     = module.kubernetes.app_namespace
  domain_suffix = var.domain_suffix
  app_image_tag = var.app_image_tag

  # Database (conditional on managed DB)
  database_host                  = var.enable_managed_database ? module.database[0].host : ""
  database_port                  = var.enable_managed_database ? module.database[0].port : 5432
  database_user                  = var.enable_managed_database ? module.database[0].user : "sub2api"
  database_password              = var.enable_managed_database ? module.database[0].password : ""
  database_name                  = var.enable_managed_database ? module.database[0].database : "sub2api"
  in_cluster_postgresql_password = random_password.sub2api_postgresql_password.result
  in_cluster_redis_password      = random_password.sub2api_redis_password.result

  # Secrets (auto-generated)
  jwt_secret          = random_password.jwt_secret.result
  totp_encryption_key = random_id.totp_encryption_key.hex
  admin_email         = var.admin_email
  admin_password      = random_password.admin_password.result

  depends_on = [module.kubernetes]
}

# --- Monitoring (optional) ---

module "monitoring" {
  source = "../modules/monitoring"
  count  = var.enable_monitoring ? 1 : 0

  chart_path    = "${path.module}/../../deploy/helm/monitoring"
  domain_suffix = var.domain_suffix

  grafana_admin_password = random_password.grafana_admin_password.result

  # R2 storage (from storage module)
  r2_endpoint   = var.enable_observability_storage ? module.storage[0].s3_endpoint : ""
  r2_access_key = var.r2_access_key
  r2_secret_key = var.r2_secret_key
  tempo_bucket  = var.enable_observability_storage ? module.storage[0].tempo_bucket : ""
  loki_bucket   = var.enable_observability_storage ? module.storage[0].loki_bucket : ""

  depends_on = [module.kubernetes]
}
