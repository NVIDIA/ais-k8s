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

> **Note:** `container_runtime` customizes cAdvisor volume mounts (accepted values are `docker`, `crio`, and `containerd`). 
Additional Alloy chart Helm values can be set in the Helm values file linked in each Helmfile environment's `valuesPath`.

The alloy-config chart contains the alloy specification for a full pipeline to scrape logs and metrics depending on the values provided.
See the values for the [default environment](./config/alloy-config/default.yaml) for explanation of some options. 
Components will be created as needed depending on which local or remote exporter values are set.

The `ethtool` value sets `device_include` on the unix exporter's ethtool collector (disabled when unset). 
`metrics_include` is optional; when unset, the chart uses a default regex limited to NIC-health counters. 
The collector reads driver-level NIC counters and needs the `NET_ADMIN` capability, which is not added by default. 
See [the default alloy-config values](./config/alloy-config/default.yaml) for the values to set. 

For internal deployment values, see the `ais-infra` repo.

# Authentication

For remote writes, you'll need to authenticate with a secret.

Refer to the secrets-operator README in the `ais-infra` repo for setup information.

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

## Verified HTTPS scraping

Set `scrape.https: true` and provide the public CA certificate through a ConfigMap:

```yaml
scrape:
  https: true
  tls:
    caConfigMap: aistore.nvidia.com
    caNamespace: monitoring
    caKey: ca.crt
    serverName: ais-target.ais
```

The ConfigMap must exist and contain the CA certificate under the specified key; no CA private key is needed.
`serverName` is optional when the discovered scrape address is covered by the server certificate.
Discovery currently uses pod IPs, so set it to a DNS name covered by every scraped server certificate when those IPs are not in the certificate.
It changes the TLS verification name and SNI, not the scrape destination.

For a public certificate served on the scraped port, use its public DNS name (matching `spec.tls.public.serverName`), not an internal service name.
This chart still requires a ConfigMap containing the trusted CA certificates; system-trust-only configuration is not added here.
Changing discovery to per-pod DNS names is separate from this setting and is not implemented here.

For compatibility, setting `scrape.https: true` without `scrape.tls` keeps the previous `insecure_skip_verify = true` behavior: traffic is encrypted, but the server certificate and hostname are not verified.
Providing `scrape.tls` enables certificate verification; use this for production HTTPS scraping.
Helmfile environments can supply the `scrape` block under `alloyConfigOverrides` without changing the remote values file.
See [Grafana's scrape TLS reference](https://grafana.com/docs/alloy/latest/reference/components/prometheus/prometheus.scrape/#tls_config) for the underlying settings.
