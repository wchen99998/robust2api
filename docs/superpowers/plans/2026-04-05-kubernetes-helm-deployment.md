# Kubernetes Helm Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a Helm chart that deploys Sub2API (app + PostgreSQL + Redis) to Kubernetes with a single `helm install`.

**Architecture:** Monorepo Helm chart at `deploy/helm/sub2api/` with Bitnami PostgreSQL and Redis as optional subcharts. Environment-based config via ConfigMap + Secret. Ingress with optional TLS.

**Tech Stack:** Helm 3, Kubernetes 1.33+, Bitnami PostgreSQL/Redis subcharts, OrbStack for local testing.

**Spec:** `docs/superpowers/specs/2026-04-05-kubernetes-helm-deployment-design.md`

---

## File Structure

```
deploy/helm/sub2api/
├── Chart.yaml
├── values.yaml
├── values-production.yaml
├── .helmignore
├── templates/
│   ├── _helpers.tpl
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── ingress.yaml
│   ├── serviceaccount.yaml
│   ├── pvc.yaml
│   └── NOTES.txt
```

---

### Task 1: Chart.yaml and .helmignore

**Files:**
- Create: `deploy/helm/sub2api/Chart.yaml`
- Create: `deploy/helm/sub2api/.helmignore`

- [ ] **Step 1: Create Chart.yaml**

```yaml
apiVersion: v2
name: sub2api
description: Sub2API - AI API Gateway Platform for subscription quota distribution
type: application
version: 0.1.0
appVersion: "latest"

dependencies:
  - name: postgresql
    version: "~16"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: postgresql.enabled
  - name: redis
    version: "~20"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: redis.enabled
```

Note: The `~16` and `~20` are approximate version constraints for the latest Bitnami chart major versions. The exact versions will be locked in `Chart.lock` after `helm dependency build`.

- [ ] **Step 2: Create .helmignore**

```
# Patterns to ignore when building packages
.DS_Store
.git
.gitignore
.vscode
*.swp
*.bak
*.tmp
*~
charts/*.tgz
```

- [ ] **Step 3: Build dependencies**

Run: `cd /Users/chenwuhao/Dev/sub2api/deploy/helm/sub2api && helm dependency build`

Expected: Downloads postgresql and redis chart tarballs into `charts/`, creates `Chart.lock`.

- [ ] **Step 4: Add charts/ to .gitignore**

Append to `deploy/helm/sub2api/.helmignore` — the tarballs are already ignored. But also ensure `deploy/helm/sub2api/charts/*.tgz` is covered. The `Chart.lock` file SHOULD be committed (it locks versions).

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/sub2api/Chart.yaml deploy/helm/sub2api/.helmignore deploy/helm/sub2api/Chart.lock
git commit -m "feat(helm): add Chart.yaml with PostgreSQL and Redis subchart dependencies"
```

---

### Task 2: Template Helpers (_helpers.tpl)

**Files:**
- Create: `deploy/helm/sub2api/templates/_helpers.tpl`

- [ ] **Step 1: Create _helpers.tpl**

```yaml
{{/*
Expand the name of the chart.
*/}}
{{- define "sub2api.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "sub2api.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart label value.
*/}}
{{- define "sub2api.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "sub2api.labels" -}}
helm.sh/chart: {{ include "sub2api.chart" . }}
{{ include "sub2api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "sub2api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sub2api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name.
*/}}
{{- define "sub2api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "sub2api.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database host: subchart service or external.
*/}}
{{- define "sub2api.databaseHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" .Release.Name }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Database port.
*/}}
{{- define "sub2api.databasePort" -}}
{{- if .Values.postgresql.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.externalDatabase.port | toString }}
{{- end }}
{{- end }}

{{/*
Database user.
*/}}
{{- define "sub2api.databaseUser" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.user }}
{{- end }}
{{- end }}

{{/*
Database name.
*/}}
{{- define "sub2api.databaseName" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Database SSL mode.
*/}}
{{- define "sub2api.databaseSSLMode" -}}
{{- if .Values.postgresql.enabled }}
{{- "disable" }}
{{- else }}
{{- default "require" .Values.externalDatabase.sslmode }}
{{- end }}
{{- end }}

{{/*
Redis host: subchart service or external.
*/}}
{{- define "sub2api.redisHost" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis-master" .Release.Name }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Redis port.
*/}}
{{- define "sub2api.redisPort" -}}
{{- if .Values.redis.enabled }}
{{- "6379" }}
{{- else }}
{{- .Values.externalRedis.port | toString }}
{{- end }}
{{- end }}

{{/*
Secret name: existing or chart-managed.
*/}}
{{- define "sub2api.secretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- include "sub2api.fullname" . }}
{{- end }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/templates/_helpers.tpl
git commit -m "feat(helm): add template helpers with DB/Redis host resolution"
```

---

### Task 3: ConfigMap

**Files:**
- Create: `deploy/helm/sub2api/templates/configmap.yaml`

- [ ] **Step 1: Create configmap.yaml**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
data:
  AUTO_SETUP: "true"
  SERVER_HOST: "0.0.0.0"
  SERVER_PORT: {{ .Values.config.serverPort | quote }}
  SERVER_MODE: {{ .Values.config.serverMode | quote }}
  RUN_MODE: {{ .Values.config.runMode | quote }}
  TZ: {{ .Values.config.timezone | quote }}
  # Database (non-sensitive)
  DATABASE_HOST: {{ include "sub2api.databaseHost" . | quote }}
  DATABASE_PORT: {{ include "sub2api.databasePort" . | quote }}
  DATABASE_USER: {{ include "sub2api.databaseUser" . | quote }}
  DATABASE_DBNAME: {{ include "sub2api.databaseName" . | quote }}
  DATABASE_SSLMODE: {{ include "sub2api.databaseSSLMode" . | quote }}
  DATABASE_MAX_OPEN_CONNS: {{ .Values.config.database.maxOpenConns | quote }}
  DATABASE_MAX_IDLE_CONNS: {{ .Values.config.database.maxIdleConns | quote }}
  DATABASE_CONN_MAX_LIFETIME_MINUTES: {{ .Values.config.database.connMaxLifetimeMinutes | quote }}
  DATABASE_CONN_MAX_IDLE_TIME_MINUTES: {{ .Values.config.database.connMaxIdleTimeMinutes | quote }}
  # Redis (non-sensitive)
  REDIS_HOST: {{ include "sub2api.redisHost" . | quote }}
  REDIS_PORT: {{ include "sub2api.redisPort" . | quote }}
  REDIS_DB: {{ .Values.config.redis.db | quote }}
  REDIS_POOL_SIZE: {{ .Values.config.redis.poolSize | quote }}
  REDIS_MIN_IDLE_CONNS: {{ .Values.config.redis.minIdleConns | quote }}
  REDIS_ENABLE_TLS: {{ .Values.config.redis.enableTLS | quote }}
  # Security
  SECURITY_URL_ALLOWLIST_ENABLED: {{ .Values.config.security.urlAllowlistEnabled | quote }}
  SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP: {{ .Values.config.security.urlAllowlistAllowInsecureHTTP | quote }}
  SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS: {{ .Values.config.security.urlAllowlistAllowPrivateHosts | quote }}
  {{- if .Values.config.security.urlAllowlistUpstreamHosts }}
  SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS: {{ .Values.config.security.urlAllowlistUpstreamHosts | quote }}
  {{- end }}
  {{- if .Values.config.updateProxyURL }}
  UPDATE_PROXY_URL: {{ .Values.config.updateProxyURL | quote }}
  {{- end }}
  {{- range $key, $value := .Values.extraEnv }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/templates/configmap.yaml
git commit -m "feat(helm): add ConfigMap template for non-sensitive configuration"
```

---

### Task 4: Secret

**Files:**
- Create: `deploy/helm/sub2api/templates/secret.yaml`

- [ ] **Step 1: Create secret.yaml**

The secret is only created when `existingSecret` is not set.

```yaml
{{- if not .Values.existingSecret }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
type: Opaque
stringData:
  {{- if .Values.postgresql.enabled }}
  DATABASE_PASSWORD: {{ .Values.postgresql.auth.password | quote }}
  {{- else }}
  DATABASE_PASSWORD: {{ .Values.externalDatabase.password | quote }}
  {{- end }}
  {{- if .Values.redis.enabled }}
  REDIS_PASSWORD: {{ .Values.redis.auth.password | quote }}
  {{- else }}
  REDIS_PASSWORD: {{ .Values.externalRedis.password | quote }}
  {{- end }}
  JWT_SECRET: {{ .Values.secrets.jwtSecret | quote }}
  JWT_EXPIRE_HOUR: {{ .Values.secrets.jwtExpireHour | quote }}
  TOTP_ENCRYPTION_KEY: {{ .Values.secrets.totpEncryptionKey | quote }}
  ADMIN_EMAIL: {{ .Values.secrets.adminEmail | quote }}
  ADMIN_PASSWORD: {{ .Values.secrets.adminPassword | quote }}
  {{- if .Values.secrets.geminiOAuthClientID }}
  GEMINI_OAUTH_CLIENT_ID: {{ .Values.secrets.geminiOAuthClientID | quote }}
  {{- end }}
  {{- if .Values.secrets.geminiOAuthClientSecret }}
  GEMINI_OAUTH_CLIENT_SECRET: {{ .Values.secrets.geminiOAuthClientSecret | quote }}
  {{- end }}
  {{- if .Values.secrets.geminiCliOAuthClientSecret }}
  GEMINI_CLI_OAUTH_CLIENT_SECRET: {{ .Values.secrets.geminiCliOAuthClientSecret | quote }}
  {{- end }}
  {{- if .Values.secrets.antigravityOAuthClientSecret }}
  ANTIGRAVITY_OAUTH_CLIENT_SECRET: {{ .Values.secrets.antigravityOAuthClientSecret | quote }}
  {{- end }}
  {{- range $key, $value := .Values.extraSecretEnv }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/templates/secret.yaml
git commit -m "feat(helm): add Secret template with existingSecret support"
```

---

### Task 5: PVC and ServiceAccount

**Files:**
- Create: `deploy/helm/sub2api/templates/pvc.yaml`
- Create: `deploy/helm/sub2api/templates/serviceaccount.yaml`

- [ ] **Step 1: Create pvc.yaml**

```yaml
{{- if .Values.persistence.enabled }}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
spec:
  accessModes:
    {{- toYaml .Values.persistence.accessModes | nindent 4 }}
  {{- if .Values.persistence.storageClass }}
  storageClassName: {{ .Values.persistence.storageClass | quote }}
  {{- end }}
  resources:
    requests:
      storage: {{ .Values.persistence.size | quote }}
{{- end }}
```

- [ ] **Step 2: Create serviceaccount.yaml**

```yaml
{{- if .Values.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "sub2api.serviceAccountName" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/sub2api/templates/pvc.yaml deploy/helm/sub2api/templates/serviceaccount.yaml
git commit -m "feat(helm): add PVC and ServiceAccount templates"
```

---

### Task 6: Deployment

**Files:**
- Create: `deploy/helm/sub2api/templates/deployment.yaml`

- [ ] **Step 1: Create deployment.yaml**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "sub2api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/configmap: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
      labels:
        {{- include "sub2api.labels" . | nindent 8 }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "sub2api.serviceAccountName" . }}
      securityContext:
        fsGroup: 1000
      terminationGracePeriodSeconds: 30
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
          envFrom:
            - configMapRef:
                name: {{ include "sub2api.fullname" . }}
            - secretRef:
                name: {{ include "sub2api.secretName" . }}
          startupProbe:
            httpGet:
              path: /health
              port: http
            failureThreshold: 30
            periodSeconds: 2
          livenessProbe:
            httpGet:
              path: /health
              port: http
            periodSeconds: 30
            timeoutSeconds: 10
          readinessProbe:
            httpGet:
              path: /health
              port: http
            periodSeconds: 10
            timeoutSeconds: 5
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
          {{- if .Values.persistence.enabled }}
          volumeMounts:
            - name: data
              mountPath: /app/data
          {{- end }}
      {{- if .Values.persistence.enabled }}
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: {{ include "sub2api.fullname" . }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/templates/deployment.yaml
git commit -m "feat(helm): add Deployment template with probes, security context, config checksums"
```

---

### Task 7: Service and Ingress

**Files:**
- Create: `deploy/helm/sub2api/templates/service.yaml`
- Create: `deploy/helm/sub2api/templates/ingress.yaml`

- [ ] **Step 1: Create service.yaml**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
      name: http
  selector:
    {{- include "sub2api.selectorLabels" . | nindent 4 }}
```

- [ ] **Step 2: Create ingress.yaml**

```yaml
{{- if .Values.ingress.enabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
  {{- with .Values.ingress.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- if .Values.ingress.className }}
  ingressClassName: {{ .Values.ingress.className | quote }}
  {{- end }}
  {{- if .Values.ingress.tls.enabled }}
  tls:
    - hosts:
        - {{ .Values.ingress.host | quote }}
      secretName: {{ default (printf "%s-tls" (include "sub2api.fullname" .)) .Values.ingress.tls.secretName }}
  {{- end }}
  rules:
    - host: {{ .Values.ingress.host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "sub2api.fullname" . }}
                port:
                  name: http
{{- end }}
```

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/sub2api/templates/service.yaml deploy/helm/sub2api/templates/ingress.yaml
git commit -m "feat(helm): add Service and Ingress templates with optional TLS"
```

---

### Task 8: values.yaml

**Files:**
- Create: `deploy/helm/sub2api/values.yaml`

- [ ] **Step 1: Create values.yaml**

```yaml
# -- Number of app replicas
replicaCount: 1

image:
  # -- Container image repository
  repository: ghcr.io/wchen99998/sub2api
  # -- Image tag (defaults to Chart appVersion)
  tag: ""
  # -- Image pull policy
  pullPolicy: IfNotPresent

# -- Image pull secrets for private registries
imagePullSecrets: []

# -- Override chart name
nameOverride: ""
# -- Override full release name
fullnameOverride: ""

serviceAccount:
  # -- Create a ServiceAccount
  create: true
  # -- ServiceAccount annotations
  annotations: {}
  # -- ServiceAccount name (auto-generated if empty)
  name: ""

# -- Use an existing Secret instead of creating one
# The secret must contain: DATABASE_PASSWORD, REDIS_PASSWORD, JWT_SECRET,
# TOTP_ENCRYPTION_KEY, ADMIN_EMAIL, ADMIN_PASSWORD
existingSecret: ""

# =============================================================================
# Application Configuration (non-sensitive, stored in ConfigMap)
# =============================================================================
config:
  serverPort: "8080"
  serverMode: "release"
  runMode: "standard"
  timezone: "Asia/Shanghai"

  database:
    maxOpenConns: 50
    maxIdleConns: 10
    connMaxLifetimeMinutes: 30
    connMaxIdleTimeMinutes: 5

  redis:
    db: 0
    poolSize: 1024
    minIdleConns: 10
    enableTLS: false

  security:
    urlAllowlistEnabled: false
    urlAllowlistAllowInsecureHTTP: false
    urlAllowlistAllowPrivateHosts: false
    urlAllowlistUpstreamHosts: ""

  updateProxyURL: ""

# -- Extra environment variables added to ConfigMap (key-value pairs)
extraEnv: {}

# =============================================================================
# Secrets (sensitive, stored in Secret)
# =============================================================================
secrets:
  jwtSecret: ""
  jwtExpireHour: "24"
  totpEncryptionKey: ""
  adminEmail: "admin@sub2api.local"
  adminPassword: ""
  geminiOAuthClientID: ""
  geminiOAuthClientSecret: ""
  geminiCliOAuthClientSecret: ""
  antigravityOAuthClientSecret: ""

# -- Extra secret environment variables (key-value pairs)
extraSecretEnv: {}

# =============================================================================
# Persistence (/app/data volume)
# =============================================================================
persistence:
  enabled: true
  size: 1Gi
  storageClass: ""
  accessModes:
    - ReadWriteOnce

# =============================================================================
# Service
# =============================================================================
service:
  type: ClusterIP
  port: 80

# =============================================================================
# Ingress
# =============================================================================
ingress:
  enabled: true
  className: ""
  host: sub2api.local
  annotations: {}
  tls:
    enabled: false
    secretName: ""

# =============================================================================
# Resources
# =============================================================================
resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 500m
    memory: 512Mi

# =============================================================================
# Scheduling
# =============================================================================
nodeSelector: {}
tolerations: []
affinity: {}

# =============================================================================
# PostgreSQL Subchart (Bitnami)
# =============================================================================
postgresql:
  enabled: true
  auth:
    username: sub2api
    password: ""
    database: sub2api
  primary:
    persistence:
      size: 1Gi

# -- External database (used when postgresql.enabled=false)
externalDatabase:
  host: ""
  port: 5432
  user: sub2api
  password: ""
  database: sub2api
  sslmode: require

# =============================================================================
# Redis Subchart (Bitnami)
# =============================================================================
redis:
  enabled: true
  auth:
    enabled: true
    password: ""
  architecture: standalone
  master:
    persistence:
      size: 1Gi

# -- External Redis (used when redis.enabled=false)
externalRedis:
  host: ""
  port: 6379
  password: ""
  enableTLS: false
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/values.yaml
git commit -m "feat(helm): add values.yaml with full configuration surface"
```

---

### Task 9: values-production.yaml

**Files:**
- Create: `deploy/helm/sub2api/values-production.yaml`

- [ ] **Step 1: Create values-production.yaml**

```yaml
# =============================================================================
# Production Overrides
# =============================================================================
# Usage: helm install sub2api ./deploy/helm/sub2api -f ./deploy/helm/sub2api/values-production.yaml
#
# IMPORTANT: Set secrets via --set flags or external secret management, NOT in this file.
#   helm install sub2api ./deploy/helm/sub2api \
#     -f ./deploy/helm/sub2api/values-production.yaml \
#     --set secrets.jwtSecret=<value> \
#     --set secrets.totpEncryptionKey=<value> \
#     --set secrets.adminPassword=<value>
# =============================================================================

replicaCount: 2

config:
  database:
    maxOpenConns: 256
    maxIdleConns: 128

  redis:
    poolSize: 4096
    minIdleConns: 256

ingress:
  host: sub2api.example.com
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  tls:
    enabled: true

resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: "2"
    memory: 2Gi

# Disable in-cluster databases for production (use managed services)
postgresql:
  enabled: false

redis:
  enabled: false

# Configure external services
externalDatabase:
  host: your-postgres-host.example.com
  port: 5432
  user: sub2api
  password: ""
  database: sub2api
  sslmode: require

externalRedis:
  host: your-redis-host.example.com
  port: 6379
  password: ""
  enableTLS: true
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/values-production.yaml
git commit -m "feat(helm): add production values overlay with external DB/Redis and TLS"
```

---

### Task 10: NOTES.txt

**Files:**
- Create: `deploy/helm/sub2api/templates/NOTES.txt`

- [ ] **Step 1: Create NOTES.txt**

```
=======================================================
  Sub2API has been deployed!
=======================================================

Release: {{ .Release.Name }}
Namespace: {{ .Release.Namespace }}

{{- if .Values.ingress.enabled }}

Access via Ingress:
{{- if .Values.ingress.tls.enabled }}
  URL: https://{{ .Values.ingress.host }}
{{- else }}
  URL: http://{{ .Values.ingress.host }}
{{- end }}

  NOTE: You need an ingress controller installed in your cluster.
  If you don't have one, install nginx-ingress:

    helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
    helm install ingress-nginx ingress-nginx/ingress-nginx

{{- else }}

Access via port-forward:

  kubectl port-forward svc/{{ include "sub2api.fullname" . }} 8080:{{ .Values.service.port }} -n {{ .Release.Namespace }}

  Then open: http://localhost:8080

{{- end }}

Quick verification:

  kubectl get pods -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }}
  kubectl logs -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }} -f
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/sub2api/templates/NOTES.txt
git commit -m "feat(helm): add NOTES.txt with post-install instructions"
```

---

### Task 11: Lint and Template Validation

**Files:** None (validation only)

- [ ] **Step 1: Lint the chart**

Run: `helm lint ./deploy/helm/sub2api`

Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 2: Dry-run template rendering**

Run: `helm template test ./deploy/helm/sub2api --set postgresql.auth.password=test --set redis.auth.password=test`

Expected: Renders all templates without errors. Review the output to verify:
- ConfigMap has correct DATABASE_HOST (should be `test-postgresql`)
- Secret has DATABASE_PASSWORD and REDIS_PASSWORD set
- Deployment references the ConfigMap and Secret
- Ingress is rendered with host `sub2api.local`
- PVC is rendered with 1Gi

- [ ] **Step 3: Dry-run with external database**

Run: `helm template test ./deploy/helm/sub2api --set postgresql.enabled=false --set externalDatabase.host=ext-pg.example.com --set externalDatabase.password=test --set redis.enabled=false --set externalRedis.host=ext-redis.example.com --set externalRedis.password=test`

Expected: ConfigMap shows `DATABASE_HOST: ext-pg.example.com`, `REDIS_HOST: ext-redis.example.com`. No subchart resources rendered.

- [ ] **Step 4: Fix any issues found in steps 1-3**

If lint or template rendering fails, fix the templates and re-run.

---

### Task 12: Deploy and Test on Local Cluster

**Files:** None (deployment test)

- [ ] **Step 1: Create namespace**

Run: `kubectl create namespace sub2api-test`

- [ ] **Step 2: Install the chart**

Run:
```bash
helm install sub2api ./deploy/helm/sub2api \
  --namespace sub2api-test \
  --set postgresql.auth.password=testpass123 \
  --set redis.auth.password=testpass123 \
  --set secrets.jwtSecret=testjwtsecret123456789012345678 \
  --set secrets.adminPassword=admin123
```

Expected: Helm reports successful install with NOTES.txt output.

- [ ] **Step 3: Wait for pods to be ready**

Run: `kubectl get pods -n sub2api-test -w`

Expected: 3 pods (sub2api, postgresql, redis) reach `Running` status with `1/1 READY`. May take 1-2 minutes for images to pull.

- [ ] **Step 4: Verify health endpoint**

Run:
```bash
kubectl port-forward svc/sub2api -n sub2api-test 8080:80 &
sleep 2
curl -s http://localhost:8080/health
kill %1
```

Expected: Health check returns a success response.

- [ ] **Step 5: Verify helm upgrade triggers pod restart on config change**

Run:
```bash
helm upgrade sub2api ./deploy/helm/sub2api \
  --namespace sub2api-test \
  --set postgresql.auth.password=testpass123 \
  --set redis.auth.password=testpass123 \
  --set secrets.jwtSecret=testjwtsecret123456789012345678 \
  --set secrets.adminPassword=admin123 \
  --set config.timezone=UTC
```

Expected: The sub2api pod restarts (new pod created, old one terminated) because the ConfigMap checksum annotation changed.

- [ ] **Step 6: Clean up**

Run:
```bash
helm uninstall sub2api --namespace sub2api-test
kubectl delete namespace sub2api-test
```

- [ ] **Step 7: Final commit**

```bash
git add -A deploy/helm/
git commit -m "feat(helm): complete Helm chart for Kubernetes deployment

Includes:
- App deployment with health probes and security context
- Bitnami PostgreSQL and Redis as optional subcharts
- External database/Redis support for production
- Ingress with optional TLS via cert-manager
- ConfigMap/Secret with checksum-based pod restart
- Production values overlay"
```
