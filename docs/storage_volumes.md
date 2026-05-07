# Target Data Persistent Volumes

The operator manages [Persistent Volume Claims (PVCs)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) for target Pods that bind to existing PVs.
AIStore storage PVs are created **outside** the scope of an AIS cluster and are **not** managed by the operator.

## PVC Templating

The operator defines PVCs based on the AIStore custom resource spec, specifically `spec.targetSpec.mounts`.

The name for the PVC template is based on the configured mounts in the AIS spec.
Each mount defines a separate path.
The operator uses the configured mount paths to define a volume claim template for every mount.
This template is used to generate PVCs for each Pod in the target StatefulSet.
The name for the actual PVCs created for each Pod follows this pattern:

```text
<cluster name><sanitized path>-<cluster name>-target-<target ordinal>
```

Paths are sanitized by replacing all `/` with `-`.

For example, the path `/ais/nvme0n1` with a cluster named `ais` produces a PVC named `ais-ais-nvme0n1-ais-target-0` for Pod `ais-target-0`.

For simple creation of static, local HostPath-type PVs, we provide the [create-target-pv Helm chart](../helm/ais/charts/create-target-pv/README.md).

## PV Requirements

Existing PVs must have the following for AIStore PVCs to bind:

- `accessModes` field includes `ReadWriteOnce`
- `storageClassName` matches spec `mount.storageClass`
  - This can be a simple string label with no defined storage class or provisioner, e.g. `"ais-local-storage"`.
- `capacity.storage` must be at least the `mount.size` from spec
- A pre-configured filesystem (XFS recommended)
  - This may be created by the provisioner

For local PVs, set `nodeAffinity` so the Pod gets scheduled on the node that holds the disk.
With a `WaitForFirstConsumer` storage class, binding is deferred until the target Pod is scheduled, so the PV on the chosen node is the one bound.
To instead pin a PVC to a specific PV up front, set `claimRef` on the PV to match the expected PVC name and namespace (see naming convention above).

`mount.selector` in the AIStore resource is propagated onto the PVC's `spec.selector` and filters the candidate PVs by labels.
Without a `claimRef`, every PV on a node is an equally valid candidate, so the selector is what binds each PVC to the PV backed by the same host path.
The AIS Helm chart does not expose `mount.selector`.
Instead, each path gets its own label selector injected based on `mount.path`:

```yaml
selector:
  matchLabels:
    mpath: pv-{{ regexReplaceAll "[^a-z0-9.-]+" (lower .path) "-" | trimPrefix "-" | trimSuffix "-" }}
```

## End-to-End Example

The following examples use the provided [create-target-pv Helm chart](../helm/ais/charts/create-target-pv/).

### Sample PV

Below is a PV as created by the `create-target-pv` chart, where the K8s node name is the IP shown:

```text
Name:              10.49.42.56-pv-ais-nvme0n1
Labels:            cluster=ais
                   mpath=pv-ais-nvme0n1
                   type=local
Annotations:       <none>
Finalizers:        [kubernetes.io/pv-protection]
StorageClass:      ais-local-storage
Status:            Bound
Claim:             ais/ais-ais-nvme0n1-ais-target-16
Reclaim Policy:    Retain
Access Modes:      RWO
VolumeMode:        Filesystem
Capacity:          6816972092211200m
Node Affinity:
  Required Terms:
    Term 0:        kubernetes.io/hostname in [10.49.42.56]
Message:
Source:
    Type:          HostPath (bare host directory volume)
    Path:          /ais/nvme0n1
    HostPathType:
```

Bound PVC:

```text
Name:          ais-ais-nvme0n1-ais-target-16
Namespace:     ais
StorageClass:  ais-local-storage
Status:        Bound
Volume:        10.49.42.56-pv-ais-nvme0n1
Labels:        app=ais
               app.kubernetes.io/component=target
               app.kubernetes.io/name=ais
               component=target
Annotations:   pv.kubernetes.io/bind-completed: yes
               pv.kubernetes.io/bound-by-controller: yes
Finalizers:    [kubernetes.io/pvc-protection]
Capacity:      6816972092211200m
Access Modes:  RWO
VolumeMode:    Filesystem
Used By:       ais-target-16
```

### AIS Helm Values

```yaml
mpathInfo:
  storageClass: "ais-local-storage"
  size: 6.2Ti
  mounts:
    - path: /ais/nvme0n1
    - path: /ais/nvme1n1
    - path: /ais/nvme2n1
```

### AIS Resource `spec.targetSpec.mounts`

The following shows the mounts as templated by Helm into the AIStore custom resource spec:

```yaml
- path: /ais/nvme0n1
  selector:
    matchLabels:
      mpath: pv-ais-nvme0n1
  size: 6.2Ti
  storageClass: ais-local-storage
- path: /ais/nvme1n1
  selector:
    matchLabels:
      mpath: pv-ais-nvme1n1
  size: 6.2Ti
  storageClass: ais-local-storage
- path: /ais/nvme2n1
  selector:
    matchLabels:
      mpath: pv-ais-nvme2n1
  size: 6.2Ti
  storageClass: ais-local-storage
```

## HostPath Option

If a mount has the `useHostPath` field set to `true`, the operator will create a host path volume at `<mount.path>/<namespace>/<cluster name>/target` and map it directly into the Pod, bypassing PVs and PVCs entirely.

This can be used in deployments that wish to avoid PV management entirely and accept the [security implications of host path mounts](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath).
