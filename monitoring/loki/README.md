# Loki
- [Main docs](https://grafana.com/docs/loki/latest/)
- [Chart source](https://github.com/grafana/loki/tree/main/production/helm/loki)
- [Additional values options](https://grafana.com/docs/loki/latest/setup/install/helm/reference/)

# Usage

## Template a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod template`

## Sync a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync`