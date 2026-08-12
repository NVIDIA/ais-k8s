# ais-create-target-pv

Helm chart that creates hostPath PersistentVolumes for AIS target nodes.
One PV is templated per (target node, mount path), pinned to its host with `nodeAffinity` and published under a `WaitForFirstConsumer` StorageClass.

See [Target Data Persistent Volumes](../../../../docs/storage_volumes.md) for details and information on other mounting options.

## Node Discovery

Target nodes come from `nodes`, a list of `kubernetes.io/hostname` label values.
When `nodes` is empty, the chart looks up nodes labeled `<targetLabelKey>=<cluster>` and reads their `kubernetes.io/hostname` label.
Discovery needs a live cluster, so an offline `helm template` must pass `nodes` explicitly.

`helm diff` also renders client-side by default, where discovery finds nothing.
Either run `helmfile diff --args "--dry-run=server"` or set `nodes` in the environment values.

## PV Naming and Binding

Each PV is named `<node>-pv-<mount-path>`, with `/` and any other character outside `[a-z0-9.-]` collapsed to `-`.

PVs carry an `mpath: pv-<mount-path>` label. The AIS chart injects a matching label selector on each target PVC, so a PVC only considers the PVs for its own mount path.
Binding is resolved by the scheduler: it places the target Pod on a node, then the PV on that node whose label matches is bound.
No `claimRef` is set, so no node has to be mapped to a target ordinal ahead of time.

One PV per (node, mount) means one target per node. For [multiple targets per node](../../../../docs/multiple_targets_per_node.md), create the PVs separately.

## StorageClass

`createStorageClass: true` creates the StorageClass named by `mpathInfo.storageClass` with `volumeBindingMode: WaitForFirstConsumer` and `provisioner: kubernetes.io/no-provisioner`.
Set it to `false` when the StorageClass already exists, in which case it must also use `WaitForFirstConsumer`.
The StorageClass is cluster-scoped, so only one release can own a given name.

## Values

| Key                      | Default                        | Description                                                        |
|--------------------------|--------------------------------|--------------------------------------------------------------------|
| `cluster`                | (required, from shared config) | AIS cluster name, used for node label matching and PV labels       |
| `targetLabelKey`         | `nvidia.com/ais-target`        | Node label key used to discover target nodes                       |
| `nodes`                  | `[]`                           | Explicit target node hostnames, discovered by label when empty     |
| `createStorageClass`     | `true`                         | Create the StorageClass named by `mpathInfo.storageClass`          |
| `mpathInfo.storageClass` | (required, from shared config) | StorageClass for created PVs                                       |
| `mpathInfo.size`         | (required, from shared config) | Capacity of each PV                                                |
| `mpathInfo.mounts`       | (required, from shared config) | List of mount paths (each with a `path` field)                     |

## Note on uninstall

The PVs are Helm-managed, so `helm uninstall` deletes them.
A PV still bound to a target PVC stays in `Terminating` until that PVC is deleted.
The reclaim policy is `Retain`, so the data on the underlying disks is left in place.

## PV Removal

A node that is no longer discovered drops out of the rendered release, so the next sync deletes its PVs.
This applies whether the node lost the target label or the Node object itself was deleted.
A PV still bound to a target PVC stays in `Terminating` until that PVC is deleted, and will not accept a new claim in the meantime.

To remove all PVs for a given cluster, delete them manually with `kubectl delete pv -l cluster=<cluster>`.
[cleanup-data-pvs.sh](../../scripts/cleanup-data-pvs.sh) deletes the target PVCs first and then the released PVs.
Re-running `helmfile sync` recreates the PVs so they can bind again.
