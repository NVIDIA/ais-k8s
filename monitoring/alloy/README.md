Run example:

```
# Template a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e prod template

# Sync a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync

# To deploy with scraping of an HTTPS AIS cluster set the HTTPS variable

set -a; . ../oci-iad.env ; set +a; helmfile -e prod --set https=true sync
```

Default values: https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml
