# ais-create-target-pv-job

> **Deprecated:** use [ais-create-target-pv](../create-target-pv/README.md) for new clusters.
> This chart remains for clusters whose PVs it already created, which cannot be moved to the new chart in place.
> It has no helmfile release, so it must be installed directly with `helm`.

Helm chart that creates hostPath PersistentVolumes for AIS target nodes.
PVs are created by a Kubernetes Job that runs as a Helm hook on `pre-install` and `pre-upgrade` triggers.

Subsequent chart installations will be blocked until all PVs are verified as `Available` or `Bound`.

See [Target Data Persistent Volumes](../../../../docs/storage_volumes.md) for details and information on other mounting options.

## Existing PVs

Nodes with existing AIS data PVs are skipped entirely.
AIS currently requires all targets to have the same disk configuration.
If changing this, we recommend a full reconfiguration (which can involve mounting the same host paths to preserve data).

See [PV Removal](#pv-removal) for an example of how to remove existing PVs.

## PV Creation

1. A Job discovers all nodes with the label `<targetLabelKey>=<cluster>` (sorted by creation time).
2. For each node, it checks whether an AIS data PV already exists (by the deterministic PV name) and skips it if so.
3. The script checks for missing PVs for each target index from 0 up to the number of labeled target nodes.
4. For each missing (node, mount) combination, it runs `kubectl apply` in the background to create PVs concurrently.
5. After all PV creation finishes, the script verifies every new PV has reached `Available` or `Bound` status.
6. Helm waits for the Job to succeed before considering the release installed and allowing downstream charts (e.g. `ais-cluster`) to proceed.

The Job runs on both initial install and upgrades (`pre-install,pre-upgrade` hooks),
so new target nodes added after initial deployment will get PVs created on the next `helmfile sync`.
If no new nodes are found, the Job exits immediately.

## PV Naming

Each PV is named `<node>-pv-<mount-path>` with a `claimRef` matching the [AIS PVC naming convention](../../../../docs/storage_volumes.md#pvc-templating).

## Values

| Key                       | Default                        | Description                                                      |
|---------------------------|--------------------------------|------------------------------------------------------------------|
| `cluster`                 | (required, from shared config) | AIS cluster name, used for node label matching and PV/PVC naming |
| `targetLabelKey`          | `nvidia.com/ais-target`        | Node label key used to discover target nodes                     |
| `mpathInfo.storageClass`  | (required, from shared config) | StorageClass for created PVs                                     |
| `mpathInfo.size`          | (required, from shared config) | Capacity of each PV                                              |
| `mpathInfo.mounts`        | (required, from shared config) | List of mount paths (each with a `path` field)                   |
| `kubectlImage.repository` | `docker.io/aistorage/ais-util` | Container image for the Job                                      |
| `kubectlImage.tag`        | `v4.9`                         | Image tag                                                        |
| `kubectlImage.pullPolicy` | `IfNotPresent`                 | Image pull policy                                                |

## Hook lifecycle

All resources (RBAC, ConfigMap, Job) are created as Helm hooks with:
- `helm.sh/hook: pre-install,pre-upgrade` — runs on first install and on every upgrade
- `helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded` — cleaned up after success; retained on failure for debugging

## Note on uninstall

PVs created by the Job are not tracked as Helm-managed resources.
Running `helm uninstall` will not delete them.

## PV Removal

To remove all PVs for a given cluster, delete them manually with `kubectl delete pv -l cluster=<cluster>`.
Note that these are created as hostPath PVs, so the data on the underlying disks will not be deleted by PV deletion.

## Target Indices

Each PV is reserved for one target through its `claimRef`, so every node holding PVs owns a target index.
The Job derives the taken indices by reading the `claimRef` names of existing PVs, then assigns the free indices in `0` to `N-1` in ascending order, where `N` is the number of labeled target nodes.
Freed indices are reused, so an index released in the middle of the range is filled before the range grows.

If every index is taken but a labeled node still lacks PVs, the Job reports the PVs to remove and exits with an error.

### Freeing an index

An index stays taken for as long as PVs claim it, including after the target's pod is gone.
Deleting the PVCs alone is not enough: the PVs move to `Released` and keep the `claimRef` of the deleted PVC.

To release index `<n>`, delete its PVCs and then its PVs:

```bash
kubectl delete pv -l cluster=<cluster>,target-index=<n>
```

PVs created before the `target-index` label was added do not match that selector.
Their `claimRef` still names the claim, so select on it instead:

```bash
kubectl get pv -l cluster=<cluster> --no-headers \
  -o custom-columns=NAME:.metadata.name,CLAIM:.spec.claimRef.name \
  | awk '$2 ~ /-ais-target-<n>$/ {print $1}' \
  | xargs -r kubectl delete pv
```

The next run assigns that index to a labeled node without PVs.
