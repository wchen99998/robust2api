locals {
  use_external_db = var.database_host != ""
  ingress_host    = "sub2api-${var.namespace}.${var.domain_suffix}"
  tls_secret_name = "sub2api-${var.namespace}-tls"
}

resource "null_resource" "helm_deps" {
  triggers = {
    chart_lock = filemd5("${var.chart_path}/Chart.lock")
  }

  provisioner "local-exec" {
    command = "helm dependency build ${var.chart_path}"
  }
}

resource "helm_release" "sub2api" {
  name             = "sub2api"
  chart            = var.chart_path
  namespace        = var.namespace
  create_namespace = false
  wait             = true
  timeout          = 600

  values = [
    file("${var.chart_path}/values-production.yaml")
  ]

  # --- Image ---
  set {
    name  = "image.tag"
    value = var.app_image_tag
  }

  # --- Ingress ---
  set {
    name  = "ingress.host"
    value = local.ingress_host
  }

  set {
    name  = "ingress.tls.secretName"
    value = local.tls_secret_name
  }

  # --- Database ---
  set {
    name  = "postgresql.enabled"
    value = local.use_external_db ? "false" : "true"
  }

  set_sensitive {
    name  = "postgresql.auth.password"
    value = var.in_cluster_postgresql_password
  }

  dynamic "set" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.host"
      value = var.database_host
    }
  }

  dynamic "set" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.port"
      value = tostring(var.database_port)
    }
  }

  dynamic "set" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.user"
      value = var.database_user
    }
  }

  dynamic "set_sensitive" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.password"
      value = var.database_password
    }
  }

  dynamic "set" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.database"
      value = var.database_name
    }
  }

  dynamic "set" {
    for_each = local.use_external_db ? [1] : []
    content {
      name  = "externalDatabase.sslmode"
      value = "require"
    }
  }

  # --- Redis (always in-cluster for now) ---
  set {
    name  = "redis.enabled"
    value = "true"
  }

  set_sensitive {
    name  = "redis.auth.password"
    value = var.in_cluster_redis_password
  }

  # --- Secrets ---
  set_sensitive {
    name  = "secrets.jwtSecret"
    value = var.jwt_secret
  }

  set_sensitive {
    name  = "secrets.totpEncryptionKey"
    value = var.totp_encryption_key
  }

  set {
    name  = "secrets.adminEmail"
    value = var.admin_email
  }

  set_sensitive {
    name  = "secrets.adminPassword"
    value = var.admin_password
  }

  depends_on = [null_resource.helm_deps]
}
