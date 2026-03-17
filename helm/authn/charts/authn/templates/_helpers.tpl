{{/*
Chart label value.
*/}}
{{- define "authn.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Selector labels (immutable, used by Deployment matchLabels and Service selectors).
Preserves backward compatibility with existing selector: app.kubernetes.io/name = .Release.Name
*/}}
{{- define "authn.selectorLabels" -}}
app.kubernetes.io/name: {{ .Release.Name }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "authn.labels" -}}
helm.sh/chart: {{ include "authn.chart" . }}
{{ include "authn.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
