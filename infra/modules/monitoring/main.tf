locals {
  grafana_host = "grafana-monitoring.${var.domain_suffix}"
}

resource "null_resource" "helm_deps" {
  triggers = {
    chart_lock = filemd5("${var.chart_path}/Chart.lock")
  }

  provisioner "local-exec" {
    command = "helm dependency build ${var.chart_path}"
  }
}

resource "helm_release" "monitoring" {
  name             = "monitoring"
  chart            = var.chart_path
  namespace        = "monitoring"
  create_namespace = true
  wait             = true
  timeout          = 900

  # --- Grafana ---
  set_sensitive {
    name  = "kube-prometheus-stack.grafana.adminPassword"
    value = var.grafana_admin_password
  }

  set {
    name  = "grafanaIngress.host"
    value = local.grafana_host
  }

  # --- Tempo (R2 storage) ---
  set {
    name  = "tempo.tempo.storage.trace.s3.bucket"
    value = var.tempo_bucket
  }

  set {
    name  = "tempo.tempo.storage.trace.s3.endpoint"
    value = var.r2_endpoint
  }

  set_sensitive {
    name  = "tempo.tempo.storage.trace.s3.access_key"
    value = var.r2_access_key
  }

  set_sensitive {
    name  = "tempo.tempo.storage.trace.s3.secret_key"
    value = var.r2_secret_key
  }

  # --- Loki (R2 storage) ---
  set {
    name  = "loki.loki.storage.s3.endpoint"
    value = var.r2_endpoint
  }

  set_sensitive {
    name  = "loki.loki.storage.s3.accessKeyId"
    value = var.r2_access_key
  }

  set_sensitive {
    name  = "loki.loki.storage.s3.secretAccessKey"
    value = var.r2_secret_key
  }

  set {
    name  = "loki.loki.storage.bucketNames.chunks"
    value = var.loki_bucket
  }

  set {
    name  = "loki.loki.storage.bucketNames.ruler"
    value = var.loki_bucket
  }

  set {
    name  = "loki.loki.storage.bucketNames.admin"
    value = var.loki_bucket
  }

  depends_on = [null_resource.helm_deps]
}
