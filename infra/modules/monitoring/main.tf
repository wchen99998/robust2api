locals {
  grafana_host     = "${var.hostname_prefix}.${var.domain_suffix}"
  r2_endpoint_host = trimsuffix(trimprefix(trimprefix(var.r2_endpoint, "https://"), "http://"), "/")
  grafana_pg_enabled = (
    var.grafana_db_host != "" &&
    var.grafana_db_name != "" &&
    var.grafana_db_user != "" &&
    var.grafana_db_password != ""
  )
}

resource "null_resource" "helm_deps" {
  triggers = {
    chart_lock = filemd5("${var.chart_path}/Chart.lock")
  }

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      for attempt in 1 2 3; do
        if helm dependency build "${var.chart_path}"; then
          exit 0
        fi
        if [ "$attempt" -lt 3 ]; then
          echo "helm dependency build failed for ${var.chart_path}, retrying ($${attempt}/3)..." >&2
          sleep $((attempt * 2))
        fi
      done
      echo "helm dependency build failed for ${var.chart_path} after 3 attempts" >&2
      exit 1
    EOT
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

  # --- Grafana PostgreSQL datasource ---
  set {
    name  = "grafanaPostgresDatasource.enabled"
    value = local.grafana_pg_enabled ? "true" : "false"
  }

  set {
    name  = "grafanaPostgresDatasource.host"
    value = var.grafana_db_host
  }

  set {
    name  = "grafanaPostgresDatasource.port"
    value = tostring(var.grafana_db_port)
  }

  set {
    name  = "grafanaPostgresDatasource.database"
    value = var.grafana_db_name
  }

  set {
    name  = "grafanaPostgresDatasource.user"
    value = var.grafana_db_user
  }

  set {
    name  = "grafanaPostgresDatasource.sslmode"
    value = var.grafana_db_sslmode
  }

  set_sensitive {
    name  = "grafanaPostgresDatasource.password"
    value = var.grafana_db_password
  }

  # --- Tempo (R2 storage) ---
  set {
    name  = "tempo.tempo.storage.trace.s3.bucket"
    value = var.tempo_bucket
  }

  set {
    name  = "tempo.tempo.storage.trace.s3.endpoint"
    value = local.r2_endpoint_host
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
    value = local.r2_endpoint_host
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
