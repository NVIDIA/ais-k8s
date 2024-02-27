# This directory is still under development -- [Ansible playbooks](../../playbooks/README.md) are the ONLY currently supported deployment mechanism

## Deploying AIS with Helm (requires operator)

This directory contains Helm charts for deploying AIS along with its dependencies. It assumes a properly configured K8s cluster with drives already formatted and mounted for use (see the [host-config ansible playbooks](../../playbooks/host-config/README.md)).

## K8s Jobs

For administrative tasks such as labeling nodes, creating persistent volumes, and managing node-local files, we use K8s Jobs managed by their own Helm charts. 

## Prerequisites

1. Kubernetes nodes configured with formatted drives
1. Helm installed locally
1. Local Kubectl configured to access the cluster

## Usage

The `ais` directory contains the chart and values for a complete AIS deployment. Updating all the values and installing this chart will run each of the sub-charts in `charts`, overriding their values. If you need to run an individual chart, the charts in `charts` also contain their own chart definitions and sample values.yaml (see [below](#individual-charts)).

### Create Namespaces

Before creating resources with Helm, ensure the namespaces are created for your cluster and K8s jobs. These can be the same namespace or separate ones. 

```bash
kubectl create ns <example>
```

### Update Values

Next, copy the `example-values.yaml` file in the directory for the chart you want to run.
Then modify the values in your new file for your desired cluster. 

### Install Charts

To install the charts provided, reference the `values.yaml` configured for your deployment. Then from the `ais` directory run: 

```bash 
helm install -f values.yaml ais .
```

To upgrade: 

```bash
helm upgrade -f values.yaml ais .
```

To uninstall:
```bash
helm uninstall ais
```

## Individual Charts

If you only want to modify one part of the installation, run the charts individually in `./ais/charts` with their own `values.yaml` files.

| Chart             | Description                                                                                       |
|-------------------|---------------------------------------------------------------------------------------------------|
| [ais-helper-rbac](./ais/charts/helper-rbac/Chart.yaml)   | Deploys role-based access control for K8s jobs.            |
| [ais-label-nodes](./ais/charts/label-nodes/Chart.yaml)  | Labels nodes according to their desired role.               |
| [ais-create-pv](./ais/charts/create-pv/Chart.yaml)  | Creates persistent volumes to be used by AIS targets.           |
