# AIStore K8s Deployment Guide

This document provides comprehensive, step-by-step guidance for deploying [AIStore](https://github.com/NVIDIA/aistore) clusters on Kubernetes (K8s).

## Contents

1. [**Prerequisites**](#prerequisites)
1. [**Deployment Steps**](#deployment-steps)
   - [Operator Deployment](#operator-deployment)
   - [AIStore Deployment](#aistore-deployment)
1. [**Post-Deployment Steps**](#post-deployment-steps)
   - [Setting Up a Debugging Pod](#setting-up-a-debugging-pod)
   - [Monitoring](#monitoring)
   - [Performance Testing with aisloader](#performance-testing-with-aisloader)
1. [**Troubleshooting Help**](#troubleshooting)

## Prerequisites

Generally, any recent version of K8s on a recent Linux OS will be sufficient for AIS. 
See the [prerequisites doc](./prerequisites.md) to ensure your cluster is ready.

For network setup details, see the [network configuration doc](./network_configuration.md).

- **Ansible Host Config Playbooks**
To assist you in setting up your system for AIStore, we've included a set of [Ansible playbooks](../playbooks/host-config/README.md) for host configuration.
For an effective initial setup, we suggest following the [`ais_host_config_common guide`](../playbooks/host-config/docs/ais_host_config_common.md).
This will help you fine-tune your system to meet AIStore's requirements, ensuring optimal performance. 

- **Persistent Volumes**:
  - The AIS Operator does **NOT** format disks or create persistent volumes -- we expect this to be done beforehand as it varies per deployment. 
  - Refer to the [prerequisites doc](./prerequisites.md) for formatting disks.
  - For creating PVs in Helm deployments see the [Helm README](../helm/README.md#pv-creation) 
  - For creating PVs in Ansible deployments, the `create_pv` playbook is an optional part of the AIS deployment playbook. Refer to the [AIS playbook docs](../playbooks/ais-deployment/docs/ais_cluster_management.md#2-deploying-aistore) 

## Deployment Steps

**Note:** Please refer to the [compatibility matrix](COMPATIBILITY.md) for AIStore and ais-operator. We recommend and only support the latest versions for both.

### Operator Deployment

With Kubernetes installed and the nodes properly configured, it's time to deploy the [AIS Operator](../operator/README.md).

- Refer to the [ais_cluster_management](../playbooks/ais-deployment/docs/ais_cluster_management.md) for detailed deployment instructions with Ansible playbooks.

#### Operator Deployment Options:

1. **Helm Chart** -- Refer to the [AIS Helm docs](../helm/README.md)
2. **Ansible Playbook (scripted release yaml)** -- Use the ais_deploy_operator playbook as detailed [here](../playbooks/ais-deployment/docs/ais_cluster_management.md#1-deploying-ais-kubernetes-operator)
3. **Local build (custom builds, development, and testing)** -- Refer to the [AIS Operator docs](../operator/README.md#deploy-ais-operator)


After deployment, use `kubectl` to check the status of the deployed pods:
  ```
  $ kubectl get pods -n ais-operator-system
  ```
  The AIS Operator pod should be in the `Running` state, indicating a successful deployment.

Once deployed, the AIS Operator will reconcile the state of any deployed AIStore custom resources.

### AIStore Deployment

With the AIS Operator deployed, the next step is to configure and deploy an AIStore custom resource.
Again, there are a few deployment options:

1. **Helm Charts (recommended)** -- See [AIS Helm Charts](../helm/README.md)
2. **Ansible Playbooks** -- Follow the [guide here](../playbooks/ais-deployment/docs/ais_cluster_management.md#2-deploying-aistore) to use the [`ais_deploy_cluster.yml`](../playbooks/ais-deployment/ais_deploy_cluster.yml) playbook.
3. **Manual resource creation (advanced)**
    - If you want to manage everything yourself, it is possible to create the required namespace, PVs, secrets, and AIStore custom resource separately.
    - The AIS Operator will create all the other K8s resources based on the AIS spec (configmaps, statefulsets, services, pods, etc.).
    - Reference our [samples](./samples/), [helm template](../helm/ais/charts/ais-cluster/templates/ais.yaml), and commands used in the [ansible playbooks](../playbooks/ais-deployment/).

**Multihome Deployment**:
  - For a multihome deployment using multiple network interfaces, some extra configuration is required before deploying the cluster.
  - Refer to the [multihome deployment doc](../playbooks/ais-deployment/docs/deploy_with_multihome.md) for details. 

After deployment, verify all AIS pods are in the `Running` stage:
```
$ watch kubectl get pods -n <cluster-namespace>
```

> **Notes**
> - In some Kubernetes deployments, the default cluster domain name might differ from `cluster.local` which can be overridden using the `clusterDomain` spec option.
> - For production environments, it's recommended to operate one proxy and one target per Kubernetes (K8s) node as shown in the above playbooks. [Multiple storage targets](multiple_targets_per_node.md) can also be deployed on a single K8s node for testing or higher availability.

### Configuring access

See the [operator docs](../operator/README.md#enabling-external-access) for configuring external access to AIS proxies and targets.

## Post-Deployment Steps

### Setting Up a Debugging Pod

Deploying a debug pod can be useful for troubleshooting and verifying cluster functionality.
To get started, we provide a simple pod spec, [aisnode_debug.yaml](../manifests/debug/aisnode_debug.yaml).
This file contains the configuration for a lightweight debug container.
Deploy it into the `ais` namespace with the following command:

```bash
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/ais-k8s/main/manifests/debug/aisnode_debug.yaml
```
After the pod is deployed, give it a moment to initialize. Once ready, access the pod using:

```bash
kubectl -n ais exec -it pods/aisnode-debug -- bash
```

Inside the debug pod, set up the environment to connect to the AIStore cluster.
You can do this by setting the `AIS_ENDPOINT` environment variable to the proxy's service address:

```bash
export AIS_ENDPOINT=http://ais-proxy:51080
```

The pod comes with the `ais` [CLI (command-line interface)](https://github.com/NVIDIA/aistore/blob/main/docs/cli.md) preinstalled.
You can utilize the `ais` CLI to interact with your cluster.
For example, to view the cluster's status, you can run:

```bash
ais cluster show
```

### Monitoring

AIStore supports a `/metrics` endpoint to provide prometheus metrics and outputs logs using a sidecar container to K8s standard logging interface. See the [AIS docs on metrics](https://github.com/NVIDIA/aistore/blob/main/docs/metrics.md) and [reference metrics](https://github.com/NVIDIA/aistore/blob/main/docs/metrics-reference.md).

We also provide Helm charts for configuring our monitoring stack as a starting point or reference: [Monitoring Resources](../monitoring/README.md).

### Performance Testing with aisloader

For evaluating the performance of your AIS cluster, we provide the [aisloader](https://github.com/NVIDIA/aistore/blob/main/docs/aisloader.md) load generation tool.
Additionally, [`aisloader-composer`](https://github.com/NVIDIA/aistore/tree/main/bench/tools/aisloader-composer) includes a variety of scripts and Ansible playbooks for running `aisloader` across multiple hosts.

## Troubleshooting

If you encounter any problems during the deployment process, feel free to report them on the [AIStore repository's issues page](https://github.com/NVIDIA/aistore/issues). We welcome your feedback and queries to enhance the deployment experience. 

We also provide a [troubleshooting doc](troubleshooting.md) for steps to resolve some of the issues you might come across. 

Happy deploying! 🎉🚀🖥️