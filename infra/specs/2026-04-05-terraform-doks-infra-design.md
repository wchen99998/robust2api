# Terraform DigitalOcean Kubernetes Infrastructure

## Overview

Provision a DigitalOcean Kubernetes (DOKS) cluster with supporting infrastructure (optional managed PostgreSQL, Cloudflare DNS, in-cluster bootstrapping) using Terraform. Redis runs in-cluster via the existing Bitnami Helm subchart.

## Directory Structure

```
infra/
├── modules/
│   ├── doks/           # DOKS cluster + autoscaling node pool
│   ├── database/       # Optional DO Managed PostgreSQL
│   ├── dns/            # Cloudflare A record
│   └── kubernetes/     # In-cluster bootstrap (ingress-nginx, cert-manager, namespace)
├── production/
│   ├── main.tf         # Composes all modules
│   ├── variables.tf    # All input variables
│   ├── outputs.tf      # Cluster endpoint, LB IP, DB connection
│   ├── versions.tf     # Provider version constraints
│   ├── terraform.tfvars.example
│   └── .terraform.lock.hcl
└── README.md
```

Future environments (staging) copy `production/` and override tfvars. Modules are reused unchanged.

## Module: doks

**Resources:**
- `digitalocean_kubernetes_cluster` — single cluster
- Default node pool with autoscaling

**Configuration:**
| Variable | Default | Description |
|----------|---------|-------------|
| `region` | `sgp1` | DO region |
| `cluster_name` | `sub2api` | Cluster name |
| `k8s_version` | `1.31` | Kubernetes version prefix (latest patch auto-selected) |
| `node_size` | `s-2vcpu-4gb` | Droplet size for worker nodes |
| `min_nodes` | `1` | Autoscaler minimum |
| `max_nodes` | `3` | Autoscaler maximum |
| `auto_upgrade` | `true` | Auto-upgrade patch versions |
| `surge_upgrade` | `true` | Zero-downtime node upgrades |

**Outputs:** cluster ID, endpoint, kubeconfig (sensitive), CA certificate, cluster URN.

## Module: kubernetes

Bootstraps cluster essentials. Depends on `doks` module outputs for provider configuration.

**Installs:**
1. **ingress-nginx** — Helm chart. Creates DO load balancer. Configured with `externalTrafficPolicy: Local` and DO load balancer annotations.
2. **cert-manager** — Helm chart + `ClusterIssuer` resource for Let's Encrypt production. Wires up the `cert-manager.io/cluster-issuer: letsencrypt-prod` annotation already used in the Sub2API Helm values-production.yaml.
3. **App namespace** — creates `sub2api` namespace.

**Does NOT install:** Sub2API, Redis, or in-cluster PostgreSQL (all managed via the existing Helm chart, deployed separately).

**Outputs:** Load balancer IP address.

## Module: database (Optional)

Created only when `enable_managed_database = true` (defaults to `false`).

**Resources:**
- `digitalocean_database_cluster` — PostgreSQL 16, single-node
- `digitalocean_database_db` — `sub2api` database
- `digitalocean_database_user` — `sub2api` user
- `digitalocean_database_firewall` — restricts access to DOKS cluster VPC

**Configuration:**
| Variable | Default | Description |
|----------|---------|-------------|
| `enable_managed_database` | `false` | Toggle managed DB creation |
| `db_size` | `db-s-1vcpu-1gb` | Database droplet size |
| `db_engine_version` | `16` | PostgreSQL major version |
| `region` | inherited | Same region as cluster |

**Outputs:** host, port, user, password (sensitive), database name, SSL mode. Values map directly to the Sub2API Helm chart `externalDatabase` block.

When disabled, the Bitnami PostgreSQL subchart in the existing Helm chart is used instead.

## Module: dns (Cloudflare)

**Resources:**
- `cloudflare_record` — A record pointing domain to ingress LB IP

**Configuration:**
| Variable | Default | Description |
|----------|---------|-------------|
| `cloudflare_api_token` | required | API token (sensitive) |
| `cloudflare_zone_id` | required | Zone ID for the domain |
| `record_name` | required | DNS record name (e.g. `api` or `sub2api`) |
| `record_value` | required | LB IP from kubernetes module |
| `proxied` | `true` | Enable Cloudflare proxy (CDN/WAF) |

**Does NOT manage:** The Cloudflare zone itself, SSL settings, or page rules.

## Production Environment Root

`infra/production/main.tf` composes all modules:

```hcl
module "doks"       # → modules/doks/
module "kubernetes"  # → modules/kubernetes/  (depends_on: doks)
module "dns"        # → modules/dns/          (uses: kubernetes LB IP)
module "database"   # → modules/database/     (independent, conditional)
```

**Required providers:** digitalocean, cloudflare, kubernetes, helm.

### Variables (terraform.tfvars)

| Variable | Sensitive | Description |
|----------|-----------|-------------|
| `do_token` | yes | DigitalOcean API token |
| `cloudflare_api_token` | yes | Cloudflare API token |
| `cloudflare_zone_id` | no | Cloudflare zone ID |
| `domain_name` | no | DNS record name (e.g. `api`) |
| `region` | no | DO region, defaults `sgp1` |
| `enable_managed_database` | no | Toggle managed PG, defaults `false` |

`terraform.tfvars` is gitignored. A `terraform.tfvars.example` with placeholders is committed.

### Outputs

- Cluster endpoint URL
- `doctl` kubeconfig command for local access
- Load balancer IP
- Database connection details (when managed DB enabled)

## Terraform State

Local state for now. The `.terraform/` directory and `*.tfstate*` files are gitignored. Can migrate to DigitalOcean Spaces (S3-compatible) or Terraform Cloud later.

## Dependency Flow

```
doks ──→ kubernetes ──→ dns
              │
              └── (LB IP)

database (independent, optional)
```

## Redis Strategy

Redis runs in-cluster via the Bitnami Redis Helm subchart (already a dependency in `deploy/helm/sub2api/Chart.yaml`). Starting as `architecture: standalone`. Future migration to cluster mode is a Helm values change, not a Terraform concern.

## What Terraform Manages vs. What It Does Not

| Terraform manages | Deployed separately |
|-------------------|-------------------|
| DOKS cluster + node pool | Sub2API application (Helm) |
| Ingress-nginx controller | Redis (Helm subchart) |
| cert-manager + ClusterIssuer | In-cluster PostgreSQL (Helm subchart) |
| Cloudflare DNS record | Application secrets |
| Managed PostgreSQL (optional) | CI/CD pipelines |
| `sub2api` namespace | |

## Future Considerations

- **Staging environment:** Copy `production/` to `staging/`, override tfvars with smaller sizing (`min_nodes=1`, `max_nodes=1`, no managed DB).
- **State backend migration:** Move to DO Spaces with state locking when team grows.
- **Terragrunt:** Consider if 3+ environments make DRY config worthwhile.
- **GitHub Actions:** Add `terraform plan` on PR, `terraform apply` on merge to main.
