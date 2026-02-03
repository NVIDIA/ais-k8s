{{- if not .Values.namespaceScope }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding