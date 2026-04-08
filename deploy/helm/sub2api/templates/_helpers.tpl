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
Resolve a public service host using an explicit override or the shared
<service>-<namespace>.<baseDomain> convention.
*/}}
{{- define "sub2api.publicServiceHostname" -}}
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
{{- define "sub2api.publicServiceURL" -}}
{{- $host := include "sub2api.publicServiceHostname" . | trim -}}
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
{{- define "sub2api.gatewayHost" -}}
{{- include "sub2api.publicServiceHostname" (dict "service" "gateway" "namespace" .Release.Namespace "host" .Values.ingress.gateway.host "baseDomain" .Values.public.baseDomain) -}}
{{- end }}

{{/*
Resolved primary control ingress host.
*/}}
{{- define "sub2api.controlHost" -}}
{{- include "sub2api.publicServiceHostname" (dict "service" "app" "namespace" .Release.Namespace "host" .Values.ingress.control.host "baseDomain" .Values.public.baseDomain) -}}
{{- end }}

{{/*
Resolved public gateway URL.
*/}}
{{- define "sub2api.gatewayPublicURL" -}}
{{- $override := trim (default "" .Values.config.gatewayUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if and .Values.ingress.enabled .Values.ingress.gateway.enabled -}}
{{- include "sub2api.publicServiceURL" (dict "service" "gateway" "namespace" .Release.Namespace "host" .Values.ingress.gateway.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.gateway.scheme "tlsEnabled" .Values.ingress.gateway.tls.enabled) -}}
{{- end -}}
{{- end }}

{{/*
Resolved public control/frontend URL.
*/}}
{{- define "sub2api.controlPublicURL" -}}
{{- $override := trim (default "" .Values.config.frontendUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if and .Values.ingress.enabled .Values.ingress.control.enabled -}}
{{- include "sub2api.publicServiceURL" (dict "service" "app" "namespace" .Release.Namespace "host" .Values.ingress.control.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.control.scheme "tlsEnabled" .Values.ingress.control.tls.enabled) -}}
{{- end -}}
{{- end }}

{{/*
Resolved public Grafana URL used by the control app.
*/}}
{{- define "sub2api.grafanaPublicURL" -}}
{{- $override := trim (default "" .Values.config.grafanaUrl) -}}
{{- if $override -}}
{{- $override -}}
{{- else if .Values.public.grafana.enabled -}}
{{- include "sub2api.publicServiceURL" (dict "service" "grafana" "namespace" (.Values.public.grafana.namespace | default "monitoring") "host" .Values.public.grafana.host "baseDomain" .Values.public.baseDomain "scheme" .Values.public.grafana.scheme "tlsEnabled" .Values.public.grafana.tls.enabled) -}}
{{- end -}}
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
Runtime secret name: existing secret or chart-managed runtime secret for gateway/worker.
*/}}
{{- define "sub2api.runtimeSecretName" -}}
{{- if .Values.existingSecret }}
{{- .Values.existingSecret }}
{{- else }}
{{- printf "%s-runtime" (include "sub2api.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Control-plane secret name: existing control secret or chart-managed control secret.
*/}}
{{- define "sub2api.controlSecretName" -}}
{{- if .Values.existingControlSecret }}
{{- .Values.existingControlSecret }}
{{- else }}
{{- printf "%s-control" (include "sub2api.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Bootstrap Job name. Include the release revision so Helm creates a fresh Job
on each upgrade instead of trying to patch an immutable Job spec.
*/}}
{{- define "sub2api.bootstrapJobName" -}}
{{- printf "%s-bootstrap-r%d" (include "sub2api.fullname" .) .Release.Revision | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Gateway component labels.
*/}}
{{- define "sub2api.gateway.selectorLabels" -}}
{{ include "sub2api.selectorLabels" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{/*
Control component labels.
*/}}
{{- define "sub2api.control.selectorLabels" -}}
{{ include "sub2api.selectorLabels" . }}
app.kubernetes.io/component: control
{{- end }}

{{/*
Origins allowed in iframe embeds rendered by the control frontend.
The resolved Grafana public URL origin is included automatically.
*/}}
{{- define "sub2api.frameSrcOrigins" -}}
{{- $origins := list -}}
{{- range .Values.control.frontend.extraFrameSrcOrigins }}
  {{- $trimmed := trim . -}}
  {{- if $trimmed }}
    {{- $origins = append $origins $trimmed -}}
  {{- end }}
{{- end }}
{{- $grafanaURL := include "sub2api.grafanaPublicURL" . | trim -}}
{{- if $grafanaURL }}
  {{- $parsed := urlParse $grafanaURL -}}
  {{- if and $parsed.scheme $parsed.host }}
    {{- $origins = append $origins (printf "%s://%s" $parsed.scheme $parsed.host) -}}
  {{- end }}
{{- end }}
{{- join " " ($origins | uniq) -}}
{{- end }}

{{/*
Worker component labels.
*/}}
{{- define "sub2api.worker.selectorLabels" -}}
{{ include "sub2api.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}
