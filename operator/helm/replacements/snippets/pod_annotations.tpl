      {{- with .Values.controllerManager.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
