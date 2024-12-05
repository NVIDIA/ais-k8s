Run example:

```
# Template a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e oci-iad template

# Sync a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e oci-iad sync

```

Default values: https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml
