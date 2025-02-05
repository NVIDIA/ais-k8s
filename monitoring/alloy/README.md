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

It's unlikely we'll want to manage different deployments of alloy itself, so for now the alloy deployment will load the [values provided in the default environment](./environments/default/alloy-values.yaml). 

However, it's possible the `config-chart` will need environment specific changes so for this we provide multiple environment options (currently prod, local, and remote). 
This allows for deploying alloy configs with different components. 
Currently, local only writes to the local prometheus/loki, remote only writes to an environment-configured remote write location, and prod writes to both.  

# Usage

## Template a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod template`

## Sync a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync`

## Sync a new environment with scraping for an HTTPS AIS cluster

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod --set https=true sync`