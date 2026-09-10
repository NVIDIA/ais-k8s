{{/*
Chart label value.
*/}}
{{- define "aisauth-secrets.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Labels for the resources this chart owns.
*/}}
{{- define "aisauth-secrets.labels" -}}
helm.sh/chart: {{ include "aisauth-secrets.chart" . }}
app.kubernetes.io/name: authn
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "aisauth-secrets.adminSecretName" -}}
{{- .Values.adminSecretName | default "ais-authn-su-creds" -}}
{{- end -}}

{{- define "aisauth-secrets.hmacSecretName" -}}
{{- .Values.hmacSecretName | default "ais-authn-jwt-signing-key" -}}
{{- end -}}

{{- define "aisauth-secrets.rsaPassphraseSecretName" -}}
{{- .Values.rsaPassphraseSecretName | default "ais-authn-rsa-passphrase" -}}
{{- end -}}
