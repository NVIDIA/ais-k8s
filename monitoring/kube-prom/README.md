Run example:

```
# Template a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e prod template

# Sync a new environment

set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync

```