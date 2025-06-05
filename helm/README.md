# Helm AIS Deployment
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/aistore)](https://artifacthub.io/packages/search?repo=aistore)

Helm provides a simple way of deploying AIStore (AIS) managed by the [AIS operator](../operator/README.md).
This directory contains Helm charts for deploying AIS, the AIS operator, and AIS dependencies.

Before deploying, ensure that your Kubernetes nodes are properly configured and ready for AIS deployment. 
The [host-config playbooks](../playbooks/host-config/README.md) provide a good starting point for properly configuring your hosts and formatting drives.

For deploying AIS without Helm, see the [Ansible playbooks](../playbooks/README.md). 
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

## Install Charts

To install the charts provided, we use [helmfile](https://github.com/helmfile/helmfile?tab=readme-ov-file).

To create a release for a new environment, add it to the environments section at the top of the helmfile for each required chart.
Next, copy the `values-sample.yaml` or an existing config yaml to a new file in the `config` directory for each chart.
This new file must match the name of the new environment.
Then modify the values in each new file for your desired deployment. 

### Install Cluster Issuer  (optional)

If you want to deploy with TLS but don't have an issuer configured, we include a [chart](./cluster-issuer/) to set up a [self-signed cluster issuer](https://cert-manager.io/docs/configuration/selfsigned/).
Cert-manager must be installed and running in the cluster for this step. 
Installing this chart first will create a cluster issuer that can be used to issue certificates for both the operator and AIS. 

Create a new environment, update your certificate values in a separate config entry, then run `helmfile sync -e <your-env>` to install the cluster issuer. 
`kubectl get clusterissuer` should show a new, non-namespaced `ca-issuer` ready. 

### Install the Operator
In the [operator](./operator/) directory, update [helmfile.yaml](./operator/helmfile.yaml) with the desired ais-operator chart version.
Create a new environment and update the config files for that environment. 
Install the chart with helmfile:

```bash 
helmfile sync -e <your-env>
```

> **Note**: Only operator versions >= 1.4.1 are supported via Helm Chart. See the [playbook docs](../playbooks/ais-deployment/docs/ais_cluster_management.md#1-deploying-ais-kubernetes-operator) for deploying older versions via Ansible Playbooks. 

Verify the operator installation by checking that the pod is in 'Ready' state:
```bash 
kubectl get pods -n ais-operator-system
```

### Install AIS

See the [AIS chart docs](./ais/README.md)

