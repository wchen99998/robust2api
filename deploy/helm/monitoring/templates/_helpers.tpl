{{/*
Resolve the effective namespace for public monitoring services.
*/}}
{{- define "sub2api-monitoring.publicNamespace" -}}
{{- default .Release.Namespace .Values.namespace -}}
{{- end }}

{{/*
Resolve a public service host using an explicit override or the shared
<service>-<namespace>.<baseDomain> convention.
*/}}
{{- define "sub2api-monitoring.publicServiceHostname" -}}
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
{{- define "sub2api-monitoring.publicServiceURL" -}}
{{- $host := include "sub2api-monitoring.publicServiceHostname" . | trim -}}
{{- if $host -}}
{{- $scheme := ternary "https" "http" .tlsEnabled -}}
{{- printf "%s://%s" $scheme $host -}}
{{- end -}}
{{- end }}

{{/*
Resolved Grafana host.
*/}}
{{- define "sub2api-monitoring.grafanaHost" -}}
{{- include "sub2api-monitoring.publicServiceHostname" (dict "service" "grafana" "namespace" (include "sub2api-monitoring.publicNamespace" .) "host" .Values.grafanaIngress.host "baseDomain" .Values.public.baseDomain) -}}
{{- end }}

{{/*
Resolved Grafana public URL.
*/}}
{{- define "sub2api-monitoring.grafanaPublicURL" -}}
{{- include "sub2api-monitoring.publicServiceURL" (dict "service" "grafana" "namespace" (include "sub2api-monitoring.publicNamespace" .) "host" .Values.grafanaIngress.host "baseDomain" .Values.public.baseDomain "tlsEnabled" .Values.grafanaIngress.tls.enabled) -}}
{{- end }}
