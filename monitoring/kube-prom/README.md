# Kube-prometheus-stack
- [Prometheus stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- Includes
   - [Prometheus](https://prometheus.io/) and the [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator)
   - [AlertManager](https://prometheus.io/docs/alerting/latest/alertmanager/)
   - [Grafana](https://grafana.com/)
   - [Node Exporter](https://github.com/prometheus/node_exporter)

### General Config

All the provided values are applied to the kube-prometheus-stack chart ([default values here](https://github.com/prometheus-community/helm-charts/blob/main/charts/kube-prometheus-stack/values.yaml)). 

The values for each environment are provided separately in the [environments](./environments/) directory as `values.yaml.gotmpl`. 
These are loaded first so that they can be reused by the generic value overrides for each component, found in [./values](./values/).

### Alerting

[AlertManager](https://prometheus.io/docs/alerting/latest/alertmanager/) supports various receivers, and you can configure them as needed. 
We include a slack alert config in the [alertmanager values file](./values/alertmanager.yaml.gotmpl). 
Refer to the [Prometheus Alerting Configuration](https://prometheus.io/docs/alerting/latest/configuration/#general-receiver-related-settings) for details on each receiver's config.

The rules themselves are provided via the `additionalPrometheusRulesMap` value option in the [alert-rules.yaml](./values/alert-rules.yaml).

# Usage

## Template a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod template`

## Sync a new environment

`set -a; . ../oci-iad.env ; set +a; helmfile -e prod sync`