data "digitalocean_kubernetes_versions" "this" {
  version_prefix = "${var.k8s_version}."
}

resource "digitalocean_kubernetes_cluster" "this" {
  name          = var.cluster_name
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.this.latest_version
  auto_upgrade  = var.auto_upgrade
  surge_upgrade = var.surge_upgrade
  tags          = var.tags

  node_pool {
    name       = "${var.cluster_name}-default"
    size       = var.node_size
    auto_scale = true
    min_nodes  = var.min_nodes
    max_nodes  = var.max_nodes
    tags       = var.tags
  }
}
