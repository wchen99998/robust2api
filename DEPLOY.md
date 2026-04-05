# Kubernetes Deployment Guide

Deploy Sub2API on DigitalOcean Kubernetes (DOKS) using Terraform for infrastructure and Helm for the application.

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.7
- [doctl](https://docs.digitalocean.com/reference/doctl/how-to/install/) (DigitalOcean CLI)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/) >= 3
- A DigitalOcean API token ([create one](https://cloud.digitalocean.com/account/api/tokens))
- A Cloudflare API token with DNS edit permissions ([create one](https://dash.cloudflare.com/profile/api-tokens))
- Your Cloudflare zone ID (found on your domain's overview page)

## 1. Provision Infrastructure

```bash
cd infra/production

# Create your config from the example
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars` with your values:

```hcl
# DigitalOcean
do_token     = "dop_v1_..."
region       = "sgp1"
cluster_name = "sub2api"

# Kubernetes bootstrap
letsencrypt_email = "admin@yourdomain.com"

# Cloudflare DNS
cloudflare_api_token = "..."
cloudflare_zone_id   = "..."
domain_name          = "api"        # creates api.yourdomain.com
cloudflare_proxied   = true

# Optional: managed PostgreSQL (default false, uses in-cluster Bitnami PG)
# enable_managed_database = true
```

Provision:

```bash
terraform init
terraform plan        # review what will be created
terraform apply       # type 'yes' to confirm
```

This creates:
- DOKS cluster with autoscaling (1-3 nodes)
- ingress-nginx controller + DO load balancer
- cert-manager with Let's Encrypt ClusterIssuer
- Cloudflare DNS A record pointing to the load balancer
- `sub2api` namespace

## 2. Configure kubectl

```bash
doctl kubernetes cluster kubeconfig save sub2api
kubectl get nodes     # verify connectivity
```

## 3. Deploy Sub2API

Generate secrets first:

```bash
JWT_SECRET=$(openssl rand -hex 32)
TOTP_KEY=$(openssl rand -hex 32)
ADMIN_PASS=$(openssl rand -base64 16)

echo "JWT_SECRET:    $JWT_SECRET"
echo "TOTP_KEY:      $TOTP_KEY"
echo "ADMIN_PASS:    $ADMIN_PASS"
```

Deploy with Helm:

```bash
helm install sub2api deploy/helm/sub2api \
  -n sub2api \
  -f deploy/helm/sub2api/values-production.yaml \
  --set ingress.host=api.yourdomain.com \
  --set secrets.jwtSecret="$JWT_SECRET" \
  --set secrets.totpEncryptionKey="$TOTP_KEY" \
  --set secrets.adminPassword="$ADMIN_PASS"
```

Verify:

```bash
kubectl -n sub2api get pods        # all pods should be Running
kubectl -n sub2api get ingress     # should show your host + LB IP
```

Your app should be accessible at `https://api.yourdomain.com`.

## 4. Using Managed PostgreSQL (Optional)

To use DigitalOcean Managed PostgreSQL instead of in-cluster:

```bash
cd infra/production

# Enable in terraform.tfvars
# enable_managed_database = true

terraform apply
```

Then update the Helm release to use the external database:

```bash
DB_HOST=$(terraform output -raw database_host)
DB_PORT=$(terraform output -raw database_port)
DB_USER=$(terraform output -raw database_user)
DB_PASS=$(terraform output -raw database_password)

helm upgrade sub2api deploy/helm/sub2api \
  -n sub2api \
  -f deploy/helm/sub2api/values-production.yaml \
  --set postgresql.enabled=false \
  --set externalDatabase.host="$DB_HOST" \
  --set externalDatabase.port="$DB_PORT" \
  --set externalDatabase.user="$DB_USER" \
  --set externalDatabase.password="$DB_PASS" \
  --set externalDatabase.database=sub2api \
  --set externalDatabase.sslmode=require \
  --reuse-values
```

## Common Operations

### Upgrade Sub2API

```bash
helm upgrade sub2api deploy/helm/sub2api \
  -n sub2api \
  --reuse-values \
  --set image.tag=<new-version>
```

### Scale the cluster

Edit `terraform.tfvars`:

```hcl
min_nodes = 2
max_nodes = 5
```

```bash
cd infra/production && terraform apply
```

### View logs

```bash
kubectl -n sub2api logs -f deployment/sub2api
```

### Check Terraform outputs

```bash
cd infra/production
terraform output                    # all outputs
terraform output load_balancer_ip   # specific output
terraform output kubeconfig_command # kubectl setup command
```

### Tear down

```bash
# Remove app first
helm uninstall sub2api -n sub2api

# Destroy infrastructure
cd infra/production
terraform destroy
```

## Architecture Overview

```
Cloudflare (DNS + CDN/WAF)
    |
DO Load Balancer
    |
ingress-nginx (TLS via cert-manager)
    |
Sub2API pods (namespace: sub2api)
    |
    +-- Redis (in-cluster, Bitnami subchart)
    +-- PostgreSQL (in-cluster or DO Managed)
```

## Terraform Modules

| Module | What it provisions |
|--------|--------------------|
| `infra/modules/doks` | DOKS cluster with autoscaling node pool |
| `infra/modules/kubernetes` | ingress-nginx, cert-manager, ClusterIssuer, app namespace |
| `infra/modules/database` | Optional DO Managed PostgreSQL with VPC firewall |
| `infra/modules/dns` | Cloudflare A record pointing to LB |

## Troubleshooting

### Pods stuck in Pending

Check node capacity — the autoscaler may need time to add nodes:

```bash
kubectl get nodes
kubectl describe pod <pod-name> -n sub2api
```

### Certificate not issuing

Check cert-manager logs and the certificate resource:

```bash
kubectl -n cert-manager logs deployment/cert-manager
kubectl -n sub2api describe certificate
kubectl -n sub2api describe certificaterequest
```

### Load balancer IP not assigned

Check the ingress-nginx service:

```bash
kubectl -n ingress-nginx get svc ingress-nginx-controller
```

DO load balancers can take 2-3 minutes to provision.
