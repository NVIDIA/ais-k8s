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
{{- $sidecar := dict "image" (printf "%s:%s" $name $tag) -}}
{{- with $resources }}{{- $_ := set $sidecar "resources" . }}{{- end -}}
{{- dict "logSidecar" $sidecar | toYaml -}}
{{- end -}}
{{- end -}}

{{/*
Render the `auth` block on the AIStore spec, dropping entries that carry no
value. Renders nothing when no entry is left.
*/}}
{{- define "ais-cluster.auth" -}}
{{- $auth := dict -}}
{{- range $key, $value := (.Values.auth | default dict) -}}
{{- if $value }}{{- $_ := set $auth $key $value }}{{- end -}}
{{- end -}}
{{- with $auth }}
{{- toYaml . }}
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
Render state storage on the AIStore spec. Renders nothing when no state storage
value is set.
*/}}
{{- define "ais-cluster.stateStorage" -}}
{{- $stateStorage := .Values.stateStorage | default dict -}}
{{- $modes := dict -}}
{{- range $mode := list "hostPath" "pvc" "emptyDir" -}}
{{- if hasKey $stateStorage $mode }}{{- $_ := set $modes $mode (get $stateStorage $mode) }}{{- end -}}
{{- end -}}
{{- $legacy := dict -}}
{{- with .Values.hostpathPrefix }}{{- $_ := set $legacy "hostpathPrefix" . }}{{- end -}}
{{- with .Values.stateStorageClass }}{{- $_ := set $legacy "stateStorageClass" . }}{{- end -}}
{{- if $modes -}}
{{- dict "stateStorage" $modes | toYaml -}}
{{- else if $legacy -}}
{{- toYaml $legacy -}}
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

{{/*
Render the `externalAccess` block for one daemon spec. Takes the daemon's
`externalAccess` value as the context.
*/}}
{{- define "ais-cluster.externalAccess" -}}
{{- $ea := default (dict) . -}}
{{- if $ea.enabled -}}
{{- $body := dict -}}
{{- with $ea.annotations }}{{- $_ := set $body "annotations" . }}{{- end -}}
{{- dict "externalAccess" $body | toYaml -}}
{{- end -}}
{{- end -}}
