# Helm AIS Deployment

Helm provides a simple way of deploying AIStore (AIS) managed by the [AIS operator](../operator/README.md).
This directory contains Helm charts for deploying AIS, the AIS operator, and AIS dependencies.

Before deploying, ensure that your Kubernetes nodes are properly configured and ready for AIS deployment. 
The [host-config playbooks](../playbooks/host-config/README.md) provide a good starting point for properly configuring your hosts and formatting drives.

For deploying AIS without Helm, see the [Ansible playbooks](../../playbooks/README.md). 
Both approaches deploy the AIS operator, then create an AIS custom resource to specify cluster settings. 

## Prerequisites

1. [**Local Kubectl configured to access the cluster**](#kubernetes-context)
1. Kubernetes nodes configured with formatted drives
1. Helm installed locally
    1. Helm-diff plugin: `helm plugin install https://github.com/databus23/helm-diff`
    1. Helmfile: https://github.com/helmfile/helmfile?tab=readme-ov-file

### Kubernetes context
1. Configure access to your cluster with a new context. See the [k8s docs](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/).
1. Check your current context with `kubectl config current-context`
1. Set the context to your cluster with `kubectl config use-context <your-context>`

### Update Values

To create a new release, add it to the environments section at the top of `ais/helmfile.yaml`. 

Next, copy the `values-sample.yaml` file for each chart in [./ais/config/](./charts/ais-cluster/) to a new values file with the same name as the new environment. 

Then modify the values in each new file for your desired cluster. 

### Install Charts

To install the charts provided, we use [helmfile](https://github.com/helmfile/helmfile?tab=readme-ov-file). Update the `helmfile.yaml` to configure the destination namespaces and set the environment for your deployment. 

1. In the [operator](./operator/) directory, update [helmfile.yaml](./operator/helmfile.yaml) with the desired ais-operator chart version. Install the chart with helmfile:

```bash 
helmfile sync
```

> **Note**: Only operator versions >= 1.4.1 are supported via Helm Chart. See the [playbook docs](../playbooks/ais-deployment/docs/ais_cluster_management.md#1-deploying-ais-kubernetes-operator) for deploying older versions via Ansible Playbooks. 


2. Verify the operator installation by checking that the pod is in 'Ready' state:
```bash 
kubectl get pods -n ais-operator-system
```

3. From the `ais` directory, run: 

```bash 
helmfile sync --environment <your-env>
```

To uninstall:
```bash
helmfile delete --environment <your-env>
```

## Individual Charts

If you only want to modify one part of the installation, it is possible to run the charts individually in `./ais/charts` with their own `values.yaml` files.

| Chart             | Description                                                                                       |
|-------------------|---------------------------------------------------------------------------------------------------|
| [ais-operator](https://github.com/NVIDIA/ais-k8s/releases)  | Deploy the AIS operator -- our helmfile deploys the chart generated from our latest AIS operator release |
| [ais-cluster](./ais/charts/ais-cluster/Chart.yaml)  | Create an AIS cluster resource, with the expectation the operator is already deployed           |
| [ais-create-pv](./ais/charts/create-pv/Chart.yaml)  | Create persistent volumes to be used by AIS targets           |
| [tls-issuer](./ais/charts/tls-issuer/Chart.yaml)  | Create a cert-manager Issuer for self-signed certs           |
| [tls-cert](./ais/charts/tls-cert/Chart.yaml)  | Create a cert-manager certificate           |
