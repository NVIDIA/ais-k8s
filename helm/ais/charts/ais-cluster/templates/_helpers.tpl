{{/*
Render the `logSidecar` block on the AIStore spec. Fields set under the
deprecated flat `logSidecarImage` override the corresponding fields from
`logSidecar`.
*/}}
{{- define "ais-cluster.logSidecar" -}}
{{- $img := (.Values.logSidecar).image | default dict -}}
{{- $name := $img.name -}}
{{- $tag := $img.tag -}}
{{- $resources := (.Values.logSidecar).resources -}}
{{- with .Values.logSidecarImage -}}
  {{- with .name }}{{- $name = . }}{{- end -}}
  {{- with .tag }}{{- $tag = . }}{{- end -}}
  {{- with .resources }}{{- $resources = . }}{{- end -}}
{{- end -}}
{{- if and $name $tag -}}
logSidecar:
  image: "{{ $name }}:{{ $tag }}"
  {{- with $resources }}
  resources:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end -}}
{{- end -}}
