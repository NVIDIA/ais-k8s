# Helm AIS Deployment
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/aistore)](https://artifacthub.io/packages/search?repo=aistore)

Helm provides a simple way to deploy AIStore (AIS) managed by the [AIS operator](../operator/README.md).
This directory contains Helm charts for deploying the following: 

| Component                         | Description                                                                                                                                                                                                                        |
|-----------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [AIS](./ais)                      | Templates an AIStore custom resource for defining a cluster                                                                                                                                                                        |
| [AIS K8s Operator](./operator)    | Installs the AIStore [CustomResourceDefinition](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) and deploys the [AIS K8s Operator](../operator/README.md) for reconciling AIStore resources |
| [AIS authN server](./authn)       | Deploys the beta [AIS Authentication Server](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md)                                                                                                                            |
| [AIS admin client](./ais-client)  | Creates a Deployment with a client Pod provisioned for AIS cluster access (see `adminClient` in AIStore spec for integrated option)                                                                                                |
| [AIS Loader](./aisloader)         | Deploys the [aisloader](https://github.com/NVIDIA/aistore/blob/main/docs/aisloader.md) benchmark tool for testing cluster performance                                                                                              |
| [ClusterIssuer](./cluster-issuer) | Creates a simple [cert-manager SelfSigned ClusterIssuer](https://cert-manager.io/docs/configuration/selfsigned/)                                                                                                                   |

## Prerequisites

1. [**Local Kubectl configured to access the cluster**](#kubernetes-context)
1. Kubernetes nodes configured with formatted drives
   1. The [host-config playbooks](../playbooks/host-config/README.md) provide a good starting point for properly configuring your hosts and formatting drives.
1. Helm installed locally
    1. Helm-diff plugin: `helm plugin install https://github.com/databus23/helm-diff`
    1. Helmfile: https://github.com/helmfile/helmfile?tab=readme-ov-file

### Kubernetes context
1. Configure access to your cluster with a new context. See the [k8s docs](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/).
2. Check your current context: `kubectl config current-context`
3. Switch to your cluster: `kubectl config use-context <your-context>`

## Installation Steps

We use [helmfile](https://github.com/helmfile/helmfile?tab=readme-ov-file) to install the charts.

**Before starting:** For each chart you want to deploy:
1. Add your environment to the `environments` section in the helmfile
2. Copy `values-sample.yaml` (or an existing config) to a new file in the `config` directory
3. Name the new file to match your environment name  
4. Update the values for your deployment

**Follow these steps in order:** 

### 1. Install Cluster Issuer (optional - only for HTTPS)

You need a cluster issuer only if you want HTTPS:
- HTTPS AIStore cluster, OR
- AuthN with HTTPS

If you don't want HTTPS, skip this step.

We provide a [chart](./cluster-issuer/) to set up a [self-signed cluster issuer](https://cert-manager.io/docs/configuration/selfsigned/).
Before proceeding, ensure that [cert-manager](https://cert-manager.io/) is installed and all its pods are running in your cluster.  
You can verify this by running the provided [check_cert_manager.sh script](./operator/check_cert_manager.sh).

1. Go to the [`cluster-issuer`](./cluster-issuer/) directory
2. Create a new environment in the [helmfile](./cluster-issuer/helmfile.yaml)
3. Update your certificate values in a config file
4. Run: `helmfile sync -e <your-env>`

Check it worked: `kubectl get clusterissuer` should show a `ca-issuer` ready.

### 2. Deploy [AuthN](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md) Server (optional - only if you want AuthN)

You only need AuthN if you want authentication/authorization for your AIS cluster. If you don't want AuthN, skip this step.

**Important:** Run AuthN server before the operator or AIS deployment. AuthN creates resources that the operator needs to talk to the AuthN server and AIS.

See the [`authn`](./authn/) directory for instructions on deploying the AuthN server, including all options for deploying with HTTPS and other configurations.

### 3. Install the Operator

1. Go to the [operator](./operator/) directory
2. Update [helmfile.yaml](./operator/helmfile.yaml) with your desired ais-operator chart version
3. Create a new environment and update config files for that environment
4. Install: `helmfile sync -e <your-env>`

> **Note**: Only operator versions >= 1.4.1 work with Helm Charts.

Check the deployment status:
```bash 
kubectl get deployment -n ais-operator-system
```

### 4. Install AIS

See the [AIS chart docs](./ais/README.md) for detailed instructions.

