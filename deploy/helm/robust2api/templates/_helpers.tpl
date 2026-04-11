{{/*
Expand the name of the chart.
*/}}
{{- define "robust2api.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "robust2api.fullname" -}}
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
{{- define "robust2api.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "robust2api.labels" -}}
helm.sh/chart: {{ include "robust2api.chart" . }}
{{ include "robust2api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "robust2api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "robust2api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name.
*/}}
{{- define "robust2api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "robust2api.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Resolve a public service host using an explicit override or the shared
<service>-<namespace>.<baseDomain> convention.
*/}}
{{- define "robust2api.publicServiceHostname" -}}
{{- $host := trim (default "" .host) -}}
{{- if $host -}}
{{- $host -}}
{{- else -}}
{{- $service := trim (default "" .service) -}}
{{- $namespace := trim (default "" .namespace) -}}
{{- $baseDomain := trim (default "" .baseDomain) -}}
{{- if and $service $namespace $baseDomain -}}
{{- printf "%s-%s.%s" $service $namespace $baseDomain -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Resolve a public service URL from the shared host convention.
*/}}
{{- define "robust2api.publicServiceURL" -}}
{{- $host := include "robust2api.publicServiceHostname" . | trim -}}
{{- if $host -}}
{{- $scheme := lower (trim (default "" .scheme)) -}}
{{- if not $scheme -}}
{{- $scheme = ternary "https" "http" .tlsEnabled -}}
{{- end -}}
{{- printf "%s://%s" $scheme $host -}}
{{- end -}}
{{- end }}

{{/*
Resolved primary gateway ingress host.
*/}}
{{- define "robust2api.gatewayHost" -}}
{{- include "robust2api.publicServiceHostname" (dict "service" "gateway" "namespace" .Release.Namespace "host" .Values.ingress.gateway.host "baseDomain" .Values.public.baseDomain) -}}
{{- end }}

{{/*
Resolved primary control ingress host.
*/}}
{{- define "robust2api.controlHost" -}}
{{- include "robust2api.publicServiceHostname" (dict "service" "app" "namespace" .Release.Namespace "host" .Values.ingress.control.host "baseDomain" .Values.public.baseDomain) -}}
{{- end }}

{{/*
Resolved public gateway URL.
*/}}
{{- define "robust2api.gatewayPublicURL" -}}
{{- $override := trim (default "" .Values.config.gatewayUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if and .Values.ingress.enabled .Values.ingress.gateway.enabled -}}
{{- include "robust2api.publicServiceURL" (dict "service" "gateway" "namespace" .Release.Namespace "host" .Values.ingress.gateway.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.gateway.scheme "tlsEnabled" .Values.ingress.gateway.tls.enabled) -}}
{{- end -}}
{{- end }}

{{/*
Resolved public control/frontend URL.
*/}}
{{- define "robust2api.controlPublicURL" -}}
{{- $override := trim (default "" .Values.config.frontendUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if and .Values.ingress.enabled .Values.ingress.control.enabled -}}
{{- include "robust2api.publicServiceURL" (dict "service" "app" "namespace" .Release.Namespace "host" .Values.ingress.control.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.control.scheme "tlsEnabled" .Values.ingress.control.tls.enabled) -}}
{{- end -}}
{{- end }}

{{/*
Resolved public Grafana URL used by the control app.
*/}}
{{- define "robust2api.grafanaPublicURL" -}}
{{- $override := trim (default "" .Values.config.grafanaUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if .Values.public.grafana.enabled -}}
{{- include "robust2api.publicServiceURL" (dict "service" "grafana" "namespace" (.Values.public.grafana.namespace | default "monitoring") "host" .Values.public.grafana.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.grafana.scheme "tlsEnabled" .Values.public.grafana.tls.enabled) -}}
{{- end -}}
{{- end }}

{{/*
Resolve a component-specific OTEL service name while preserving the configured
base service name.
*/}}
{{- define "robust2api.otelServiceName" -}}
{{- $base := trim (default "robust2api" .base) -}}
{{- $component := trim (default "" .component) -}}
{{- if $component -}}
{{- printf "%s-%s" $base $component -}}
{{- else -}}
{{- $base -}}
{{- end -}}
{{- end }}

{{/*
Resolve OTEL resource attributes injected by the deployment.
*/}}
{{- define "robust2api.otelResourceAttributes" -}}
{{- $root := .root -}}
{{- $component := trim (default "" .component) -}}
{{- $environment := trim (default "" $root.Values.observability.otel.environment) -}}
{{- $deploymentName := printf "%s-%s" (include "robust2api.fullname" $root) $component -}}
{{- $attrs := list
  (printf "service.namespace=%s" $root.Release.Namespace)
  "service.version=$(APP_VERSION)"
  "k8s.namespace.name=$(POD_NAMESPACE)"
  "k8s.pod.name=$(POD_NAME)"
  (printf "k8s.deployment.name=%s" $deploymentName)
  (printf "robust2api.component=%s" $component)
-}}
{{- if $environment -}}
{{- $attrs = append $attrs (printf "deployment.environment=%s" $environment) -}}
{{- end -}}
{{- join "," $attrs -}}
{{- end }}

{{/*
Database host: subchart service or external.
*/}}
{{- define "robust2api.databaseHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql" .Release.Name }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Database host as resolved from Grafana in the monitoring namespace.
*/}}
{{- define "robust2api.grafanaDatasourceHost" -}}
{{- if .Values.postgresql.enabled }}
{{- printf "%s-postgresql.%s.svc" .Release.Name .Release.Namespace }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
Database port.
*/}}
{{- define "robust2api.databasePort" -}}
{{- if .Values.postgresql.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.externalDatabase.port | toString }}
{{- end }}
{{- end }}

{{/*
Database user.
*/}}
{{- define "robust2api.databaseUser" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.username }}
{{- else }}
{{- .Values.externalDatabase.user }}
{{- end }}
{{- end }}

{{/*
Database name.
*/}}
{{- define "robust2api.databaseName" -}}
{{- if .Values.postgresql.enabled }}
{{- .Values.postgresql.auth.database }}
{{- else }}
{{- .Values.externalDatabase.database }}
{{- end }}
{{- end }}

{{/*
Database SSL mode.
*/}}
{{- define "robust2api.databaseSSLMode" -}}
{{- if .Values.postgresql.enabled }}
{{- if (dig "tls" "enabled" false .Values.postgresql) }}
{{- "require" }}
{{- else }}
{{- "disable" }}
{{- end }}
{{- else }}
{{- default "require" .Values.externalDatabase.sslmode }}
{{- end }}
{{- end }}

{{/*
PostgreSQL admin user for in-cluster provisioning.
*/}}
{{- define "robust2api.postgresqlAdminUser" -}}
{{- "postgres" }}
{{- end }}

{{/*
Image used by PostgreSQL maintenance jobs.
*/}}
{{- define "robust2api.postgresqlImage" -}}
{{- $registry := default "docker.io" .Values.postgresql.image.registry -}}
{{- $repository := default "bitnami/postgresql" .Values.postgresql.image.repository -}}
{{- $digest := default "" .Values.postgresql.image.digest -}}
{{- if $digest }}
{{- printf "%s/%s@%s" $registry $repository $digest -}}
{{- else }}
{{- printf "%s/%s:%s" $registry $repository (default "17.6.0-debian-12-r4" .Values.postgresql.image.tag) -}}
{{- end }}
{{- end }}

{{/*
Redis host: subchart service or external.
*/}}
{{- define "robust2api.redisHost" -}}
{{- if .Values.redis.enabled }}
{{- printf "%s-redis-master" .Release.Name }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end }}

{{/*
Redis port.
*/}}
{{- define "robust2api.redisPort" -}}
{{- if .Values.redis.enabled }}
{{- "6379" }}
{{- else }}
{{- .Values.externalRedis.port | toString }}
{{- end }}
{{- end }}

{{/*
Runtime secret name: existing secret or chart-managed runtime secret for gateway/worker.
*/}}
{{- define "robust2api.runtimeSecretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- printf "%s-runtime" (include "robust2api.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Control-plane secret name: existing control secret or chart-managed control secret.
*/}}
{{- define "robust2api.controlSecretName" -}}
{{- if .Values.existingControlSecret }}
{{- .Values.existingControlSecret }}
{{- else }}
{{- printf "%s-control" (include "robust2api.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Bootstrap Job name.
*/}}
{{- define "robust2api.bootstrapJobName" -}}
{{- printf "%s-bootstrap" (include "robust2api.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Bootstrap rerun checksum keyed only to bootstrap-relevant inputs.
*/}}
{{- define "robust2api.bootstrapInputsChecksum" -}}
{{- $databasePassword := ternary .Values.postgresql.auth.password .Values.externalDatabase.password .Values.postgresql.enabled -}}
{{- $inputs := dict
  "bootstrapImage" (dict
    "repository" .Values.image.bootstrap.repository
    "tag" (.Values.image.bootstrap.tag | default .Chart.AppVersion)
    "pullPolicy" .Values.image.bootstrap.pullPolicy
  )
  "database" (dict
    "host" (include "robust2api.databaseHost" . | trim)
    "port" (include "robust2api.databasePort" . | trim)
    "user" (include "robust2api.databaseUser" . | trim)
    "name" (include "robust2api.databaseName" . | trim)
    "sslmode" (include "robust2api.databaseSSLMode" . | trim)
    "password" $databasePassword
  )
  "controlSeed" (dict
    "jwtSecret" .Values.secrets.jwtSecret
    "jwtExpireHour" .Values.secrets.jwtExpireHour
    "totpEncryptionKey" .Values.secrets.totpEncryptionKey
    "adminEmail" .Values.secrets.adminEmail
    "adminPassword" .Values.secrets.adminPassword
  )
  "manualRerunToken" .Values.bootstrap.manualRerunToken
-}}
{{- toJson $inputs | sha256sum -}}
{{- end }}

{{/*
Frontend edge service name used by ingress.
*/}}
{{- define "robust2api.frontendServiceName" -}}
{{- printf "%s-control" (include "robust2api.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Control API service name used by frontend proxy.
*/}}
{{- define "robust2api.controlAPIServiceName" -}}
{{- printf "%s-control-api" (include "robust2api.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Resolved control service target for frontend proxying.
*/}}
{{- define "robust2api.frontendControlServiceName" -}}
{{- $explicit := trim (default "" .Values.frontend.controlServiceName) -}}
{{- if $explicit -}}
{{- $explicit -}}
{{- else -}}
{{ include "robust2api.controlAPIServiceName" . }}
{{- end -}}
{{- end }}

{{/*
Resolved gateway service target for frontend proxying.
*/}}
{{- define "robust2api.frontendGatewayServiceName" -}}
{{- $explicit := trim (default "" .Values.frontend.gatewayServiceName) -}}
{{- if $explicit -}}
{{- $explicit -}}
{{- else -}}
{{ printf "%s-gateway" (include "robust2api.fullname" .) }}
{{- end -}}
{{- end }}

{{/*
Grafana datasource secret name.
*/}}
{{- define "robust2api.grafanaDatasourceSecretName" -}}
{{- printf "%s-grafana-datasource-postgres" (include "robust2api.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Grafana reader reconciliation job name.
*/}}
{{- define "robust2api.grafanaReaderRoleJobName" -}}
{{- printf "%s-grafana-reader-role" (include "robust2api.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Gateway component labels.
*/}}
{{- define "robust2api.gateway.selectorLabels" -}}
{{ include "robust2api.selectorLabels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Control component labels.
*/}}
{{- define "robust2api.control.selectorLabels" -}}
{{ include "robust2api.selectorLabels" . }}
app.kubernetes.io/component: control
{{- end }}

{{/*
Frontend component labels.
*/}}
{{- define "robust2api.frontend.selectorLabels" -}}
{{ include "robust2api.selectorLabels" . }}
app.kubernetes.io/component: frontend
{{- end }}

{{/*
Extract just the scheme+host origin from a URL. This keeps chart rendering
working even before Flux substitutes placeholders like ${BASE_DOMAIN} into
production HelmRelease values.
*/}}
{{- define "robust2api.urlOrigin" -}}
{{- $url := trim (default "" .) -}}
{{- if $url -}}
{{- regexFind "^[A-Za-z][A-Za-z0-9+.-]*://[^/?#[:space:]]+" $url -}}
{{- end -}}
{{- end }}

{{/*
Origins allowed in iframe embeds rendered by the control frontend.
The resolved Grafana public URL origin is included automatically.
*/}}
{{- define "robust2api.frameSrcOrigins" -}}
{{- $origins := list -}}
{{- range .Values.control.frontend.extraFrameSrcOrigins }}
  {{- $trimmed := trim . -}}
  {{- if $trimmed }}
    {{- $origins = append $origins $trimmed -}}
  {{- end }}
{{- end }}
{{- $grafanaURL := include "robust2api.grafanaPublicURL" . | trim -}}
{{- if $grafanaURL }}
  {{- $grafanaOrigin := include "robust2api.urlOrigin" $grafanaURL | trim -}}
  {{- if $grafanaOrigin }}
    {{- $origins = append $origins $grafanaOrigin -}}
  {{- end }}
{{- end }}
{{- join " " ($origins | uniq) -}}
{{- end }}

{{/*
Worker component labels.
*/}}
{{- define "robust2api.worker.selectorLabels" -}}
{{ include "robust2api.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}

{{/*
Billing component labels.
*/}}
{{- define "robust2api.billing.selectorLabels" -}}
{{ include "robust2api.selectorLabels" . }}
app.kubernetes.io/component: billing
{{- end }}
