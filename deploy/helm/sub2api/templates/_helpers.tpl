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

{{/*
Bootstrap Job name. Include the release revision so Helm creates a fresh Job
on each upgrade instead of trying to patch an immutable Job spec.
*/}}
{{- define "sub2api.bootstrapJobName" -}}
{{- printf "%s-bootstrap-r%d" (include "sub2api.fullname" .) .Release.Revision | trunc 63 | trimSuffix "-" }}
{{- end }}
