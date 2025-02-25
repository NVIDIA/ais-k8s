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

The values for the alloy deployment itself are provided as a base `base-alloy-values.yaml` with overrides available for each environment in `environments/<env>/alloy-values.yaml`. The full list of available helm values can be found [here](https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml).

The `config-chart` defines the base components used by multiple environments in [config-chart/common](./config-chart/common/). Environment specific configurations can be found in [config-chart/environments](./config-chart/environments/) (currently prod, local, and remote). 
This allows for deploying alloy configs with different components for each environment. 
Currently, local only writes to the local prometheus/loki, remote only writes to an environment-configured remote write location, and prod writes to both.  

# Usage

## Template a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod template`

## Sync a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync`

## Sync a new environment with scraping for an HTTPS AIS cluster

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod --set https=true sync`