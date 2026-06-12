# remote-exec

Helm chart for privileged, host-level access on Kubernetes nodes.
Use it to run one-off maintenance scripts across the cluster (DaemonSet) or open an interactive debug shell on a single node (Pod).

In most cases, the access here is not necessary and one of the following options is more suitable:

- For ephemeral debug containers on an existing pod, consider [kubectl debug](https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/).
- For simple node access without host-level root, prefer [node debugging](https://kubernetes.io/docs/tasks/debug/debug-cluster/kubectl-node-debug/).
- For permanent changes across entire node group or pool, configure custom images or use a [cloud-init script](../cloud-init/README.md).

> Warning! This provides root-level access to host nodes. 
> Privileged pod deployment access should be gated by Kubernetes RBAC and/or service account tokens used for kubectl/helm authentication.

Every workload runs with 
 - `hostPID`, `hostNetwork`, `hostIPC` set to `true`
 - A privileged container
 - The host root filesystem mounted at `/host` (chroot into `/host` to run commands on the node).

DaemonSets with a script run it in an **initContainer** (once per pod lifecycle).
The main container sleeps so the pod stays up for logs until you `helm uninstall`.

For workloads with `kind: pod`, pods without a script will sleep to allow `exec` commands, while pods with a defined script will terminate after completion.

## Quick start

From this directory:

```bash
# Privileged debug pod 
helm upgrade --install debug .
# On a specific node
helm upgrade --install debug . --set nodeName=10.49.42.96

kubectl exec -it debug-privileged -- bash
# To run a shell as the host, inside the container:
chroot /host bash
```

When done:

```bash
helm uninstall debug
```

Run with a values file that defines `workload.kind: daemonset` to run on multiple nodes:

```bash
# DaemonSet example preset; uninstall when finished
helm upgrade --install grow-fs . -f values-grow-fs.yaml -n kube-system
helm uninstall grow-fs -n kube-system
```

## Preset value files

| File                       | Workload  | Script                   | Purpose                                    |
|----------------------------|-----------|--------------------------|--------------------------------------------|
| `values.yaml`              | Pod       | _(none — sleep forever)_ | Generic privileged debug / `kubectl exec`  |
| `values-grow-fs.yaml`      | DaemonSet | `grow-fs.sh`             | Run `oci-growfs` on all nodes              |
| `values-ring-buffers.yaml` | DaemonSet | `ring-buffers.sh`        | Resize ethtool ring buffers on `ens300np0` |
| `values-up-kernel.yaml`    | DaemonSet | `up-kernel.sh`           | Update `kernel-uek` from `ol8_UEKR7`       |
| `values-node-doctor.yaml`  | DaemonSet | `node-doctor.sh`         | Run Node Doctor `--check` on all nodes (see `scripts/node-doctor.md`) |

## Values

| Key                | Default                        | Description                                                                                                                             |
|--------------------|--------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| `name`             | `debug-privileged`             | Kubernetes resource name (`metadata.name`). DaemonSets also get `app: <name>` labels.                                                   |
| `workload.kind`    | `pod`                          | `pod` or `daemonset`                                                                                                                    |
| `workload.script`  | `""`                           | Bash script under `scripts/` to run at start. Empty runs `sleep infinity` (debug pod). Missing file fails at `helm template` / install. |
| `nodeName`         | `""`                           | Pin a **Pod** to a specific node. Mutually exclusive with `nodeSelector`.                                                               |
| `nodeSelector`     | `{}`                           | Schedule on nodes with matching labels (DaemonSet or Pod). Mutually exclusive with `nodeName`.                                          |
| `image.repository` | `docker.io/aistorage/ais-util` | Container image                                                                                                                         |
| `image.tag`        | `v4.5`                         | Image tag                                                                                                                               |
| `image.pullPolicy` | `IfNotPresent`                 | Image pull policy                                                                                                                       |
| `tolerations`      | `[]`                           | Pod tolerations (e.g. `operator: Exists`)                                                                                               |

## Custom scripts

Add a script under `scripts/`, e.g. `scripts/my-task.sh`. Use `chroot /host` for host commands:

```bash
#!/bin/bash
set -euo pipefail
chroot /host /bin/bash -lc 'your-command-here'
```

Create a values file for the release, e.g. `values-my-task.yaml`:

```yaml
name: my-task

workload:
  kind: daemonset
  script: my-task.sh

image:
  repository: docker.io/aistorage/ais-util
  tag: v4.5
  pullPolicy: IfNotPresent

tolerations:
  - operator: Exists
```

Use `workload.kind: pod` and omit `tolerations` for a one-off debug-style pod instead of a cluster-wide DaemonSet.

Install with your values file:

```bash
helm upgrade --install my-task . -f values-my-task.yaml -n kube-system
```

Scripts are packaged into a ConfigMap and run as `/bin/bash /scripts/<script>`.

## Notes

- The main container is always named `ais-remote-exec`; DaemonSet scripts run in initContainer `run-script`.
- Pods have no extra labels; DaemonSets label pods with `app: <name>`.
- DaemonSet pod templates use `restartPolicy: Always` (required by the API). Scripts run once via the initContainer before the main container starts.
