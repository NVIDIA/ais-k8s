{{/*
Render the name of the AIS CA ConfigMap to mount, or nothing when
`tls.enabled` is false or `tls.caConfigMap` is empty.
*/}}
{{- define "aisloader-workflow-templates.caConfigMap" -}}
{{- if .Values.tls.enabled }}{{ .Values.tls.caConfigMap }}{{ end }}
{{- end -}}

{{/*
Render workflow volumes for the AIS CA bundle and the AuthN token.
*/}}
{{- define "aisloader-workflow-templates.volumes" -}}
{{- with include "aisloader-workflow-templates.caConfigMap" . }}
- name: ais-ca
  configMap:
    name: {{ . }}
    optional: true
    items:
      - key: {{ $.Values.tls.bundleFile }}
        path: {{ $.Values.tls.bundleFile }}
{{- end }}
{{- if .Values.authn.enabled }}
- name: authn-token
  emptyDir: {}
{{- end }}
{{- end -}}

{{/*
Render TLS env for any container talking to AIS or AuthN.
*/}}
{{- define "aisloader-workflow-templates.tlsEnv" -}}
{{- if include "aisloader-workflow-templates.caConfigMap" . }}
- name: AIS_CLIENT_CA
  value: /etc/ais-ca/{{ .Values.tls.bundleFile }}
{{- end }}
{{- end -}}

{{/*
Render the AIS CA volume mount. Renders nothing when no TLS CA is configured.
*/}}
{{- define "aisloader-workflow-templates.caVolumeMount" -}}
{{- if include "aisloader-workflow-templates.caConfigMap" . }}
- name: ais-ca
  mountPath: /etc/ais-ca
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Render env for a container running AIS client tools (ais, aisloader).
*/}}
{{- define "aisloader-workflow-templates.clientEnv" -}}
- name: AIS_ENDPOINT
  value: {{ .Values.ais.endpoint | quote }}
{{- include "aisloader-workflow-templates.tlsEnv" . }}
{{- if .Values.authn.enabled }}
- name: AIS_AUTHN_TOKEN_FILE
  value: /tmp/authn/token
{{- end }}
{{- end -}}

{{/*
Render volume mounts for a container running AIS client tools.
*/}}
{{- define "aisloader-workflow-templates.clientVolumeMounts" -}}
{{- include "aisloader-workflow-templates.caVolumeMount" . }}
{{- if .Values.authn.enabled }}
- name: authn-token
  mountPath: /tmp/authn
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Render the init container that logs in to AuthN and writes the token to the
shared authn-token volume.
*/}}
{{- define "aisloader-workflow-templates.authnLogin" -}}
{{- if .Values.authn.enabled }}
- name: authn-login
  image: {{ index .Values.images "ais-util" }}
  command: [ais, auth, login]
  args: ["$(AUTHN_USERNAME)", "-p", "$(AUTHN_PASSWORD)", "--file", "/tmp/authn/token"]
  env:
    - name: AIS_AUTHN_URL
      value: {{ .Values.authn.serviceURL | quote }}
    {{- with include "aisloader-workflow-templates.tlsEnv" . }}
    {{- . | trim | nindent 4 }}
    {{- end }}
    - name: AUTHN_USERNAME
      valueFrom:
        secretKeyRef:
          name: {{ .Values.authn.secretName }}
          key: username
    - name: AUTHN_PASSWORD
      valueFrom:
        secretKeyRef:
          name: {{ .Values.authn.secretName }}
          key: password
  volumeMounts:
    - name: authn-token
      mountPath: /tmp/authn
    {{- with include "aisloader-workflow-templates.caVolumeMount" . }}
    {{- . | trim | nindent 4 }}
    {{- end }}
{{- end }}
{{- end -}}

{{/*
Render the body of `templateDefaults` that Argo merges into every template of
the workflow: the AuthN login init container, plus client env and volume mounts
for container templates.
*/}}
{{- define "aisloader-workflow-templates.templateDefaults" -}}
{{- with include "aisloader-workflow-templates.authnLogin" . }}
initContainers:
  {{- . | trim | nindent 2 }}
{{- end }}
container:
  env:
    {{- include "aisloader-workflow-templates.clientEnv" . | nindent 4 }}
  {{- with include "aisloader-workflow-templates.clientVolumeMounts" . }}
  volumeMounts:
    {{- . | trim | nindent 4 }}
  {{- end }}
{{- end -}}
