# Local Test Cluster

Scripts for deploying a local AIStore environment on a [KinD](https://kind.sigs.k8s.io/) (Kubernetes in Docker) cluster.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) or [Podman](https://podman.io/)
- [KinD](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/)
- [Helmfile](https://github.com/helmfile/helmfile#installation)

## Quick Start

```bash
# Deploy with defaults -- Current releases, TLS, no authentication
./local/test-cluster.sh

# Build the operator from source and deploy
./local/test-cluster.sh --build

# Deploy with authentication enabled
./local/test-cluster.sh --auth
```

Once the cluster is ready, connect to the admin client:

```bash
kubectl exec -it -n ais deploy/ais-client -- /bin/bash
```

## Usage

```text
Usage: test-cluster.sh [options]

Options:
  -b, --build          Build the operator from source
  --reset              Re-run Kubernetes cluster setup (prereqs, certs, namespaces)
                       without recreating the cluster
  --image <image>      Use an operator image from a remote registry
                       (e.g. ghcr.io/org/ais-operator:v1.0)
  --auth               Deploy AuthN service and configure AIS with authentication
  -h, --help           Show this help message
```

`--build` and `--image` are mutually exclusive. When neither is specified the operator is deployed with the current release image.

## What It Does

`test-cluster.sh` is the main entry point. It orchestrates the full lifecycle in order:

1. **Create a KinD cluster** — 1 control-plane node + 3 worker nodes (see `kind/config.yaml`). Workers mount `/run/udev` to support OpenEBS local volumes. Skipped if the `local-test` cluster already exists.
2. **Install prerequisites** (`prereq-helmfile.yaml`) — OpenEBS for local storage, cert-manager with its CSI driver, and trust-manager for CA distribution.
3. **Configure the cluster** — deploys the cluster issuer, creates the `ais` and `ais-operator-system` namespaces, labels nodes for AIS scheduling, and applies the trust-manager bundle that distributes the CA certificate to labeled namespaces.
4. **Deploy the AIS operator** — either built from source (`--build`), pulled from a registry (`--image`), or installed via current release helm chart and default image.
5. **Deploy AuthN** (optional, `--auth`) — installs the authentication service with a generated admin password.
6. **Deploy AIStore** — applies the AIS helmfile and waits for the `aistore/ais` resource to reach `Ready` state.

On subsequent runs the script detects the existing cluster and skips creation. Use `--reset` to re-run the prerequisite and namespace setup without destroying the cluster.

## File Layout

| File                          | Purpose                                                                         |
|-------------------------------|---------------------------------------------------------------------------------|
| `test-cluster.sh`             | Main entry point — parses options and orchestrates the full deploy              |
| `cluster-setup.sh`            | Sources `start-kind.sh` and defines `install_prereqs` / `setup_cluster` helpers |
| `start-kind.sh`               | Creates the KinD cluster and verifies the kubectl context                       |
| `delete-cluster.sh`           | Tears down the `local-test` KinD cluster                                        |
| `kind/config.yaml`            | KinD cluster configuration (node topology + mounts)                             |
| `prereq-helmfile.yaml`        | Helmfile for OpenEBS, cert-manager, CSI driver, and trust-manager               |
| `manifests/trust-bundle.yaml` | trust-manager `Bundle` CR that distributes the CA cert to AIS namespaces        |

## Cleanup

```bash
./local/delete-cluster.sh
```

This deletes the `local-test` KinD cluster and all its resources.
