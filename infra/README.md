# Sub2API Infrastructure

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
doctl kubernetes cluster kubeconfig save sub2api
```

Deploy Sub2API via Helm:

```bash
helm install sub2api ../../deploy/helm/sub2api \
  -n sub2api \
  -f ../../deploy/helm/sub2api/values-production.yaml \
  --set secrets.jwtSecret=<value> \
  --set secrets.totpEncryptionKey=<value> \
  --set secrets.adminPassword=<value>
```

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
