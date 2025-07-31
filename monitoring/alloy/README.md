# Alloy
- "Grafana Alloy is an open source OpenTelemetry Collector distribution with built-in Prometheus pipelines and support for metrics, logs, traces, and profiles." -- https://github.com/grafana/alloy 
- [Main docs](https://grafana.com/docs/alloy/latest/)
- [Chart source](https://github.com/grafana/alloy/tree/main/operations/helm/charts/alloy)
- [All default values](https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml)

# Config

We deploy two charts for alloy:
1. [config-chart](./config-chart/) -- A simple locally-managed chart to deploy a configmap containing Alloy config files
1. The actual [Alloy chart](https://github.com/grafana/alloy/tree/main/operations/helm/charts/alloy) provided by Grafana

Today we expect all environment specific changes to come through the environment variables. 

The values for the alloy deployment itself are provided as a base `base-alloy-values.yaml.gotmpl` with overrides available for each environment in `environments/<env>/alloy-values.yaml`. The full list of available helm values can be found [here](https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml).

> **Note:** Container runtime specific configurations (for cAdvisor metrics) are controlled via the `CONTAINER_RUNTIME` environment variable (default: `docker`, accepted: `docker`|`crio`).

The `config-chart` defines the base components used by multiple environments in [config-chart/common](./config-chart/common/). Environment specific configurations can be found in [config-chart/environments](./config-chart/environments/) (currently prod, local, and remote). 
This allows for deploying alloy configs with different components for each environment. 
Currently, local only writes to the local prometheus/loki, remote only writes to an environment-configured remote write location, and prod writes to both.  

# Authentication

For our panoptes remote write targets, you'll need to authenticate with our vault secrets. Follow the [instructions in the vault directory](../vault/README.md) and reference the Gitlab wiki to set up a secret in your target k8s cluster. 

# Usage

## Template a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod template`

## Sync a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync`

## Sync a new environment with scraping for an HTTPS AIS cluster

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod --set https=true sync`


# Debugging

By default, the alloy deployment creates a service `alloy` in the monitoring namespace with port `12345`. 

To port-forward for debugging, you can run the following command: 

```bash
kubectl port-forward -n monitoring svc/alloy 12345:12345
```

To force a config reload to test a config update, simply make an API call to this service:

```bash
curl -X POST http://localhost:12345/-/reload
```