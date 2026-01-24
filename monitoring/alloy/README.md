# Alloy
- "Grafana Alloy is an open source OpenTelemetry Collector distribution with built-in Prometheus pipelines and support for metrics, logs, traces, and profiles." -- https://github.com/grafana/alloy 
- [Main docs](https://grafana.com/docs/alloy/latest/)
- [Chart source](https://github.com/grafana/alloy/tree/main/operations/helm/charts/alloy)
- [All default values](https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml)

# Config

We deploy two charts for alloy:
1. [alloy-config](./charts/alloy-config/) -- A simple locally-managed chart to deploy a ConfigMap containing Alloy config files
1. The actual [Alloy chart](https://github.com/grafana/alloy/tree/main/operations/helm/charts/alloy) provided by Grafana

The default values template for the alloy deployment itself can be found in [config/alloy/](./config/alloy/).
The full list of available helm values can be found [here](https://github.com/grafana/alloy/blob/main/operations/helm/charts/alloy/values.yaml).

> **Note:** Currently, the only configurable option in the default values is the container runtime, which customizes some specific volume mounts the alloy container reads for cAdvisor metrics. Accepted values: `docker` | `crio` | `containerd`.

The alloy-config chart contains the alloy specification for a full pipeline to scrape logs and metrics depending on the values provided.
See the values for the [default environment](./config/alloy-config/default.yaml) for explanation of some options. 
Components will be created as needed depending on which local or remote exporter values are set. 

For internal deployment values, see the `ais-infra` repo.

# Authentication

For remote writes, you'll need to authenticate with a secret.
Follow the [instructions in the vault directory](../vault/README.md) and reference the Gitlab wiki to set up a secret in your target k8s cluster. 

# Usage

## Template a new environment

`helmfile -e ais-qa template`

## Sync a new environment

`helmfile -e ais-qa sync`

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

To view the full config as created in the ConfigMap: 

```bash
kubectl get configmap alloy-config \
  -n monitoring \
  -o "jsonpath={.data['config\\.alloy']}" > alloy-config.yaml
```