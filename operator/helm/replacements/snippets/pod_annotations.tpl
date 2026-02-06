{{- with .Values.controllerManager.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}