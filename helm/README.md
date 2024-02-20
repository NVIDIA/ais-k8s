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

To install the charts provided, first update all the values in values.yaml for your deployment. Then from this directory run: 

```bash 
helm install -f values.yaml ais .
```

If the namespaces you specify do not exist yet, run with the `create-namespace` option:

```bash
helm install -f values.yaml ais . --create-namespace
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

If you only want to modify one part of the installation, run the charts individually in ``./ais/charts` with their own `values.yaml` files.

| Chart             | Description                                                                                      |
|-------------------|--------------------------------------------------------------------------------------------------|
| [ais-helper-rbac](./ais/charts/helper-rbac/Chart.yaml)   | Deploys role-based access control for K8s jobs.                                                  |
| [ais-label-nodes](./ais/charts/label-nodes/Chart.yaml)  | Labels nodes according to their desired role. Used for creating persistent volumes and deploying statefulsets. |
