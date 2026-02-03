{{- toYaml .Values.controllerManager.manager.args | nindent 8 }}
        {{- if .Values.namespaceScope }}
        - {{ include "ais-operator.watchNamespaces" . | quote }}
        {{- end }}