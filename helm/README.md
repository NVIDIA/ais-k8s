# This directory is still under development -- [Ansible playbooks](../../playbooks/README.md) are the ONLY currently supported deployment mechanism

## Deploying AIS with Helm (includes operator)

This directory contains Helm charts for deploying AIS along with its dependencies. It assumes a properly configured K8s cluster with drives already formatted and mounted for use (see the [host-config ansible playbooks](../../playbooks/host-config/README.md)).

## Prerequisites

1. Kubernetes nodes configured with formatted drives
1. Helm installed locally
    1. Helm-diff plugin: `helm plugin install https://github.com/databus23/helm-diff`
    1. Helmfile: https://github.com/helmfile/helmfile?tab=readme-ov-file
1. Local Kubectl configured to access the cluster

### Update Values

Next, copy the `values-sample.yaml` file in [./charts/ais-cluster](./charts/ais-cluster/), to a new values file. 
Then modify the values in your new file for your desired cluster. 

### Install Charts

To install the charts provided, we use [helmfile](https://github.com/helmfile/helmfile?tab=readme-ov-file). Update the `helmfile.yaml` to configure the destination namespaces and reference the `values.yaml` configured for your deployment. Then from the `ais` directory run: 

```bash 
helmfile upgrade
```

To uninstall:
```bash
helmfile destroy
```

## Individual Charts

If you only want to modify one part of the installation, it is possible to run the charts individually in `./charts` with their own `values.yaml` files.

| Chart             | Description                                                                                       |
|-------------------|---------------------------------------------------------------------------------------------------|
| [ais-create-pv](./charts/create-pv/Chart.yaml)  | Creates persistent volumes to be used by AIS targets.           |
