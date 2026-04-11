# Robust2API Infrastructure

Terraform modules for provisioning DigitalOcean Kubernetes infrastructure.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- A DigitalOcean API token
- A Cloudflare API token with DNS edit permissions for your zone

## Quick Start

```bash
cd production/
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your tokens and settings
terraform init
terraform plan
terraform apply
```

## After Apply

Configure kubectl:

```bash
doctl kubernetes cluster kubeconfig save robust2api
```

Deploy Robust2API via Helm:

```bash
helm install robust2api ../../deploy/helm/robust2api \
  -n robust2api \
  -f ../../deploy/helm/robust2api/values-production.yaml \
  --set config.grafanaUrl=<grafana_url_if_monitoring_enabled> \
  --set secrets.jwtSecret=<value> \
  --set secrets.totpEncryptionKey=<value> \
  --set secrets.adminPassword=<value>
```

If you enabled managed PostgreSQL, Terraform also outputs `grafana_reader_user` and `grafana_reader_password`. Copy those values into the Robust2API release secret keys `grafanaProvisioning.reader.username` and `grafanaProvisioning.reader.password` so Grafana uses the dedicated read-only account instead of the app user.

## Modules

| Module | Description |
|--------|-------------|
| `modules/doks` | DOKS cluster with autoscaling node pool |
| `modules/kubernetes` | In-cluster bootstrap: ingress-nginx, cert-manager, namespace |
| `modules/database` | Optional DO Managed PostgreSQL |
| `modules/dns` | Cloudflare DNS A record |

## Environments

| Directory | Description |
|-----------|-------------|
| `production/` | Production environment root |
