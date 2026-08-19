# AIStore Kubernetes (K8s) Node State Storage

Each of the AIS nodes running in a K8s cluster uses a small amount of local persistent storage for caching AIS internal config and state. 

In the past, this was **always** expected to mount to a directory on the host.
This was done in part because [local volumes in K8s](https://kubernetes.io/docs/concepts/storage/volumes/#local) do not support [dynamic provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/), which allows for volumes to be created for each AIS node as the AIS statefulsets scale.
The host directory is configurable with `stateStorage.hostPath.prefix` in the AIS spec.

There are a few drawbacks to using [hostPath volumes](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath). 
First, there are additional security risks, as detailed in the prior link, because of the access to the host filesystem. 
Cleanup of the files on the host also presented some challenges, which become more pressing with the development of simpler [deployment with Helm](../helm/README.md).  
We needed to implement a k8s job to scan nodes for leftover directories to prevent config contamination on subsequent deployments.
Test runners also slowly built up data without their own cleanup job (which would be more complicated, due to multiple deployments on the same cluster, parallel runs, etc.).

This all leads to the introduction of `stateStorage.pvc.storageClass` as the recommended alternative over `stateStorage.hostPath.prefix`.
`stateStorage.pvc.storageClass` can be set to any local storage class that supports dynamic provisioning. Some options we've tested are the [Rancher Local Path Provisioner](https://github.com/rancher/local-path-provisioner) and [OpenEBS Local Storage](https://openebs.io/docs/concepts/data-engines/localstorage).

When `stateStorage.pvc.storageClass` is set to a compatible storage class, the operator will automatically configure a _dynamic_ local volume.
This simplifies volume management in our StatefulSets, as volumes are automatically created and deleted according to the required persistent volume claims (PVCs).

## emptyDir

`stateStorage.emptyDir` stores state in an ephemeral [emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).
These volumes and their data are deleted when the pod is deleted or removed from a node for any reason.  
It requires no StorageClass or host directory configuration.
When using `emptyDir`, AIStore pods can be more easily migrated between nodes as there is no PVC claim tying the pod to a local PV.

In K8s, every AIS pod is configured to reach the headless in-cluster proxy service to join on startup (see [Discovery URL](https://github.com/NVIDIA/aistore/blob/main/docs/lifecycle_node.md#joining-a-cluster-discovery-url)).
When a pod is created or rescheduled, AIS nodes join and sync metadata with the primary proxy (see [Bootstrap](https://github.com/NVIDIA/aistore/blob/main/docs/ha.md#bootstrap)). 
This means a new pod re-syncs the latest cluster config, cluster map, and any other state data (not on data mounts).

For the reasons below, if local AIStore bucket data must remain persisted through potential total cluster restarts, we recommend only enabling `emptyDir` on clusters running AIStore 5.0 or later.

### Limitations

Note that with `emptyDir`, the `shutdownCluster` option is disabled. 
This is to protect against losing the cluster UUID from the cluster map when all pods are deleted.
If all pods are deleted at the same time due to any reason, all data in the state volumes is lost.
Bucket metadata lives on the data mounts and must match the UUID in the cluster map, so the UUID must not be regenerated.

In AIStore versions 5.0 or newer, the cluster map is also persisted on a data mount on each target to protect against UUID loss. 
Even if all state volumes are lost, the primary will attempt to rebuild the same cluster ID from the joining targets that maintain the cluster map (as long as they join in the configured startup window). 
See [Target Smap recovery](https://github.com/NVIDIA/aistore/blob/main/docs/ha.md#target-smap-recovery) for more info.

Note that in the scenario of all pods restarting, config set only through API rather than custom resource spec will still be lost.

## Node Affinity

A local storage class, set through `stateStorage.pvc.storageClass`, ties its pod to a node.
The provisioner creates the volume on a single node and sets a `nodeAffinity` on the PersistentVolume so it can only be used from there.
The claim makes that placement permanent, since a bound PVC's spec is immutable and cannot be pointed at a volume on another node.

Moving a pod to a different node therefore means deleting its state PVC.
The StatefulSet recreates it, and a new volume is provisioned wherever the pod is next scheduled.

Under this mode, every proxy and target has a state claim.
A target is also held by a data volume per mount path, but for a proxy the state claim is the only thing tying it to a node.

The other state storage modes place no such constraint.
`stateStorage.hostPath.prefix` mounts a host directory directly and `stateStorage.emptyDir` is ephemeral, neither of which involves a claim.
