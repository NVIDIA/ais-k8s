# Replacing a Node

This document covers moving an AIStore proxy and target off a node that has failed or is being retired, onto a replacement node.

Helm does not drive this, as it only manages the AIStore custom resource.
Rather, Kubernetes manages [assigning Pods to Nodes](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/) based on labels, affinity, and requested resources.
AIStore deploys both proxies and targets as [StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/).
Migrating a pod with a given ordinal in a StatefulSet requires a sequence of label and resource changes, including PersistentVolume re-provisioning as outlined below.

## PersistentVolumes per role

A proxy has one PersistentVolume, for state.
A target has that plus one per mount path for data.

State volumes come from `stateStorage`, and only the `pvc` mode creates a claim.
Under `hostPath` or `emptyDir` nothing pins a proxy to a node, so steps 4 through 7 concern the target alone.
See [Node State Storage](./state_storage.md).

Proxy and target ordinals are independent, so one node commonly hosts `ais-proxy-1` alongside `ais-target-0`.

## Procedure

Replacing `<old-node>` with `<new-node>`, for a proxy at ordinal `<m>` and a target at ordinal `<n>` in namespace `<namespace>`.

### 1. Drain the target through AIS

```console
ais cluster add-remove-nodes start-maintenance <node-id>
```

`<node-id>` is the target's AIS daemon ID, not the Kubernetes node name.
The command starts a rebalance that moves the target's data onto the remaining targets.
Wait for it to finish before continuing:

```console
ais show rebalance
```

See [Node lifecycle](https://github.com/NVIDIA/aistore/blob/main/docs/lifecycle_node.md#putting-a-node-in-maintenance) for maintenance, shutdown, and decommission in AIS.

### 2. Unlabel the old node

```console
helm/ais/scripts/label-nodes.sh --remove <cluster> <old-node>
```

The AIS `nodeSelector` schedules by these labels, and the PV charts discover target nodes by the same ones.
Removing them takes the node out of both.

### 3. Delete the pods

```console
kubectl delete pod -n <namespace> ais-proxy-<m> ais-target-<n>
```

The StatefulSet recreates each pod under the same ordinal, so it picks up the same `volumeClaimTemplates` PVCs.
Those are still bound to PersistentVolumes on the old node, each carrying a `nodeAffinity` for it, so a pod holding one comes back `Pending`.
The scheduler records why as a `FailedScheduling` event, shown under `Events` in `kubectl describe pod`:

```text
0/3 nodes are available: 1 node(s) didn't match PersistentVolume's node affinity,
1 node(s) were unschedulable.
```

### 4. Delete the PVCs

A bound PVC's spec is immutable, so it cannot be changed to use a volume elsewhere.
Deleting it lets the StatefulSet create a new one, which binds on the replacement node.

Read the claim names off the pod spec so the state PVC is not missed:

```console
kubectl get pod ais-target-<n> -n <namespace> \
  -o jsonpath='{.spec.volumes[*].persistentVolumeClaim.claimName}' \
  | xargs -r -n1 kubectl delete pvc -n <namespace>
```

Repeat for `ais-proxy-<m>`.

### 5. Delete the old node's data PVs

A PersistentVolume outlives its claim according to its `persistentVolumeReclaimPolicy`.
Under `Delete` it goes away with the claim and this step does not apply.
Both PV charts use `Retain`, which leaves the PV `Released` with the deleted claim still recorded, and a `Released` PV will not accept a new claim.

How they are deleted depends on where they came from:

- [create-target-pv](../helm/ais/charts/create-target-pv/README.md) templates its PVs, so a node that is no longer discovered drops out of the rendered release and its PVs are deleted on the next sync.
- [create-target-pv-job](../helm/ais/charts/create-target-pv-job/README.md) creates its PVs from a Job, outside Helm's ownership. They must be deleted directly, which is also what frees the node's target index.
- PVs provisioned outside these charts are deleted directly with `kubectl delete pv`.

### 6. Label the replacement node

```console
helm/ais/scripts/label-nodes.sh <cluster> <new-node>
```

Label before running the chart, since target nodes are discovered at run time.

### 7. Recreate the PVs

Provision volumes for the replacement node the same way the existing ones were created.
If neither chart manages them, apply the PV manifests the cluster was built with.

Both charts name PVs `<node>-pv-<mount-path>` and provision for every labeled target node, so running the one that did not create them fails on the PVs it does not own.

`create-target-pv` runs as the `ais-create-target-pv` release, which the [Helmfile](../helm/ais/helmfile.yaml) gates on `createTargetPV.enabled`.
Set it for your environment before syncing:

```console
cd helm/ais && helmfile sync --environment <your-env> --selector name=ais-create-target-pv
```

`create-target-pv-job` is a legacy chart with no Helmfile release and may be managed directly with `helm`.

### 8. Verify

```console
kubectl get pods -n <namespace> -o wide
kubectl get pvc -n <namespace>
```

Both pods should be `Running` on the replacement node with their PVCs `Bound`.
