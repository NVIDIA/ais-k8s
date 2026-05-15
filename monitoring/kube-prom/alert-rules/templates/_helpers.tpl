{{- define "ais-alert-rules.validateDiskType" -}}
{{- if not (or (eq .Values.disk.type "nvme") (eq .Values.disk.type "hdd")) -}}
{{- fail (printf "disk.type must be \"nvme\" or \"hdd\", got %q" .Values.disk.type) -}}
{{- end -}}
{{- end -}}
