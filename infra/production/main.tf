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
