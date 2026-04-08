resource "digitalocean_database_cluster" "postgres" {
  name       = "${var.cluster_name}-pg"
  engine     = "pg"
  version    = var.db_engine_version
  size       = var.db_size
  region     = var.region
  node_count = 1
  tags       = var.tags
}

resource "digitalocean_database_db" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id
  name       = var.db_name
}

resource "digitalocean_database_user" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id
  name       = var.db_user
}

resource "digitalocean_database_user" "grafana_reader" {
  cluster_id = digitalocean_database_cluster.postgres.id
  name       = var.grafana_reader_user
}

resource "null_resource" "grafana_reader_grants" {
  triggers = {
    host               = digitalocean_database_cluster.postgres.host
    port               = tostring(digitalocean_database_cluster.postgres.port)
    database           = digitalocean_database_db.app.name
    app_user           = digitalocean_database_user.app.name
    app_password_hash  = sha256(digitalocean_database_user.app.password)
    reader_user        = digitalocean_database_user.grafana_reader.name
    reader_passwd_hash = sha256(digitalocean_database_user.grafana_reader.password)
  }

  provisioner "local-exec" {
    command = <<-EOT
      set -euo pipefail
      export PGPASSWORD='${digitalocean_database_user.app.password}'
      psql "host=${digitalocean_database_cluster.postgres.host} port=${digitalocean_database_cluster.postgres.port} dbname=${digitalocean_database_db.app.name} user=${digitalocean_database_user.app.name} sslmode=require" <<'SQL'
      REVOKE ALL ON DATABASE "${digitalocean_database_db.app.name}" FROM "${digitalocean_database_user.grafana_reader.name}";
      GRANT CONNECT ON DATABASE "${digitalocean_database_db.app.name}" TO "${digitalocean_database_user.grafana_reader.name}";
      GRANT USAGE ON SCHEMA public TO "${digitalocean_database_user.grafana_reader.name}";
      REVOKE ALL ON ALL TABLES IN SCHEMA public FROM "${digitalocean_database_user.grafana_reader.name}";
      REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM "${digitalocean_database_user.grafana_reader.name}";
      GRANT SELECT ON ALL TABLES IN SCHEMA public TO "${digitalocean_database_user.grafana_reader.name}";
      GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO "${digitalocean_database_user.grafana_reader.name}";
      ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO "${digitalocean_database_user.grafana_reader.name}";
      ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON SEQUENCES TO "${digitalocean_database_user.grafana_reader.name}";
      SQL
      unset PGPASSWORD
    EOT
  }

  depends_on = [
    digitalocean_database_db.app,
    digitalocean_database_user.app,
    digitalocean_database_user.grafana_reader,
  ]
}

resource "digitalocean_database_firewall" "app" {
  cluster_id = digitalocean_database_cluster.postgres.id

  rule {
    type  = "k8s"
    value = var.doks_cluster_id
  }
}
