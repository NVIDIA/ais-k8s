# Helm charts for AIS Cluster Monitoring

## Included charts:
- [Kube-prometheus stack](./kube-prom/README.md) for local prometheus, grafana, etc.
- [Loki](./loki/README.md) for local log storage and search
- [Alloy](./alloy/README.md) for scraping, processing, and exporting observability events

## Prereqs
1. Install and configure [helm](https://helm.sh/docs/intro/install/) and [helmfile](https://helmfile.readthedocs.io/en/latest/#installation) (including configuring kubectl context for your cluster).
1. If using local storage for persistence, set up a storage class on your cluster that can handle dynamic persistent volumes. We use [Rancher's local-path-provisioner](https://github.com/rancher/local-path-provisioner) by default.
1. If using affinity rules, label nodes for scheduling monitoring pods. Set the values for affinity nodeLabelKey and nodeLabelValue for each helm chart appropriately. Example: `kubectl label node/your-node 'aistore.nvidia.com/role_monitoring=true'`.

## Deployment
1. For a locally hosted prometheus stack including grafana, start by following the instructions in the `kube-prom` directory. 
1. For locally hosted log storage, follow the instructions in the `loki` directory.
1. Finally, follow the instructions in `alloy` to deploy Grafana Alloy for scraping, processing, and forwarding both metrics and logs from various sources. 

### Environment variables 

To use sensitive variables in your deployment, provide a `*.env` file and load it when running your helmfile commands. 

Example template: 
`set -a; . ../your-env.env ; set +a; helmfile -e prod template`

Example sync: 
`set -a; . ../your-env.env ; set +a; helmfile -e prod sync`

Here are the currently referenced optional environment variables
```
GRAFANA_PASSWORD
MIMIR_LABEL
MIMIR_ENDPOINT
LOKI_LABEL
LOKI_ENDPOINT
CONTAINER_RUNTIME
```

## Accessing internal services (Prometheus, Loki, Grafana, Alloy)

The web services for Prometheus and Grafana are not directly accessible from outside the cluster.
Options include changing the service types to `NodePort` or using port-forwarding.

Default service names and ports: 

| Tool         | Service Name                          | Default Port |
|--------------|---------------------------------------|--------------|
| Prometheus   | prometheus-kube-prometheus-prometheus | 9090         |
| Grafana      | prometheus-grafana                    | 80           |
| Loki Gateway | loki-gateway                          | 80           |
| Alloy        | alloy                                 | 12345        |

### Example Instructions for Grafana.

Configure access from the host into the pod by using ONE of the following:
   - Port-forward: `kubectl port-forward --namespace monitoring service/kube-prometheus-stack-grafana 3000:80`
   - Patch the service to use NodePort: `kubectl patch svc kube-prometheus-stack-grafana -n monitoring -p '{"spec": {"type": "NodePort"}}'`
   - Create a separate NodePort or LoadBalancer service: [k8s docs](https://kubernetes.io/docs/concepts/services-networking/service/)

If needed, use an ssh tunnel to access the k8s host: `ssh -L <port>:localhost:<port> <user-name>@<ip-or-host-name>` and view `localhost:<port>`.

For Grafana, login with the admin user and the password set with the `GRAFANA_PASSWORD` environment variable


Example output: 

![Prometheus UI](images/prometheus.png)

![Grafana Dashboard](images/grafana.png)