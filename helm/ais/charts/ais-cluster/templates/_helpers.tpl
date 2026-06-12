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

{{/*
Return the effective state storage class used for validation.
*/}}
{{- define "ais-cluster.stateStorageClass" -}}
{{- $storageClass := .Values.stateStorageClass -}}
{{- $stateStorage := .Values.stateStorage | default dict -}}
{{- if hasKey $stateStorage "pvc" -}}
  {{- $storageClass = dig "pvc" "storageClass" "" $stateStorage -}}
{{- end -}}
{{- $storageClass -}}
{{- end -}}

{{/*
Render state storage on the AIStore spec. If the new stateStorage value is set,
render it as-is so it takes precedence over legacy fields. Otherwise, render the
legacy fields for backwards compatibility.
*/}}
{{- define "ais-cluster.stateStorage" -}}
{{- $stateStorage := .Values.stateStorage | default dict -}}
{{- if or (hasKey $stateStorage "hostPath") (hasKey $stateStorage "pvc") }}
stateStorage:
{{- toYaml $stateStorage | nindent 2 }}
{{- else }}
{{- with .Values.hostpathPrefix }}
hostpathPrefix: {{ . }}
{{- end }}
{{- with .Values.stateStorageClass }}
stateStorageClass: {{ . }}
{{- end }}
{{- end -}}
{{- end -}}

{{/*
Validate that the state storage class exists. Requires cluster access, so this
is skipped during templating (e.g. `helm template`).
*/}}
{{- define "ais-cluster.validateStateStorageClass" -}}
{{- $stateStorageClass := include "ais-cluster.stateStorageClass" . -}}
{{- if $stateStorageClass }}
{{- $hasClusterAccess := (lookup "v1" "Node" "" "").items }}
{{- if $hasClusterAccess }}
{{- $sc := lookup "storage.k8s.io/v1" "StorageClass" "" $stateStorageClass }}
{{- if empty $sc }}
{{- fail (printf "StorageClass '%s' for state storage not found. Please ensure the StorageClass exists before deploying." $stateStorageClass) }}
{{- end }}
{{- end }}
{{- end }}
{{- end -}}
