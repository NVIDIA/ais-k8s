{{- define "remote-exec.name" -}}
{{- default .Release.Name .Values.name | trunc 55 | trimSuffix "-" }}
{{- end }}

{{/*
Fail at render time if workload.kind is not pod or daemonset.
*/}}
{{- define "remote-exec.validateWorkloadKind" -}}
{{- if not (has .Values.workload.kind (list "pod" "daemonset")) }}
{{- fail (printf "workload.kind must be pod or daemonset, got %q" .Values.workload.kind) }}
{{- end }}
{{- end }}

{{/*
Fail at render time if script is set but not present under scripts/.
*/}}
{{- define "remote-exec.validateScript" -}}
{{- if .Values.workload.script }}
{{- $path := printf "scripts/%s" .Values.workload.script }}
{{- if not (.Files.Get $path) }}
{{- fail (printf "script %q not found in chart (expected %s)" .Values.workload.script $path) }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Fail if nodeName and nodeSelector are both set.
*/}}
{{- define "remote-exec.validateNodePlacement" -}}
{{- if and .Values.nodeName (not (empty (.Values.nodeSelector | default dict))) }}
{{- fail "nodeName and nodeSelector are mutually exclusive; use one or the other" }}
{{- end }}
{{- end }}

{{- define "remote-exec.volumeMounts" -}}
{{- if .Values.workload.script }}
- name: script
  mountPath: /scripts
  readOnly: true
{{- end }}
- name: host-root
  mountPath: /host
{{- end }}

{{- define "remote-exec.podSpec" -}}
{{- include "remote-exec.validateScript" . }}
{{- include "remote-exec.validateNodePlacement" . }}
{{- $dsWithScript := and (eq .Values.workload.kind "daemonset") .Values.workload.script }}
{{- if .Values.nodeName }}
nodeName: {{ .Values.nodeName | quote }}
{{- else if not (empty (.Values.nodeSelector | default dict)) }}
nodeSelector:
  {{- toYaml .Values.nodeSelector | nindent 2 }}
{{- end }}
hostPID: true
hostNetwork: true
hostIPC: true
{{- with .Values.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
restartPolicy: {{ if eq .Values.workload.kind "daemonset" }}Always{{ else }}Never{{ end }}
{{- if $dsWithScript }}
initContainers:
  - name: run-script
    image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
    imagePullPolicy: {{ .Values.image.pullPolicy }}
    command:
      - /bin/bash
      - /scripts/{{ .Values.workload.script }}
    securityContext:
      privileged: true
    volumeMounts:
      {{- include "remote-exec.volumeMounts" . | nindent 6 }}
{{- end }}
containers:
  - name: ais-remote-exec
    image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
    imagePullPolicy: {{ .Values.image.pullPolicy }}
    {{- if $dsWithScript }}
    command: ["sleep", "infinity"]
    {{- else if .Values.workload.script }}
    command:
      - /bin/bash
      - /scripts/{{ .Values.workload.script }}
    {{- else }}
    command: ["sleep", "infinity"]
    {{- end }}
    securityContext:
      privileged: true
    volumeMounts:
      {{- include "remote-exec.volumeMounts" . | nindent 6 }}
volumes:
  {{- if .Values.workload.script }}
  - name: script
    configMap:
      name: {{ include "remote-exec.name" . }}-script
      defaultMode: 0755
  {{- end }}
  - name: host-root
    hostPath:
      path: /
      type: Directory
{{- end }}
