# AIStore Redeployment

This document covers redeploying an AIStore cluster through the Kubernetes operator when the existing AIStore custom resource must be removed and recreated.

For a temporary stop where the same deployment will be restarted without deleting Kubernetes resources, use `shutdownCluster` instead.
When `shutdownCluster` is enabled, the operator gracefully shuts down AIS and scales the cluster to zero without deleting data or configuration.

## Cleanup Options

The AIS custom resource has two cleanup options that control what happens when the resource is deleted.

### `cleanupMetadata`

`cleanupMetadata` controls whether the operator fully decommissions the cluster and removes AIS metadata/state during deletion.

When `cleanupMetadata: false`, the operator calls the AIS shutdown API before deleting Kubernetes resources.
This preserves AIS metadata for a future deployment of the same cluster.

When `cleanupMetadata: true`, the operator calls the AIS decommission API and cleans up AIS state:

- State PVCs are deleted when `stateStorage.pvc.storageClass` is used.
- State host paths under `stateStorage.hostPath.prefix` are cleaned by operator-managed jobs when host path state storage is used.
- Target data PVCs are also deleted so that stale state and data PVC bindings are not reused inconsistently.

Use `cleanupMetadata: true` when the next deployment must start with fresh AIS state, such as when changing protocol between HTTP and HTTPS.

### `cleanupData`

`cleanupData` controls whether AIS should delete user data, including buckets and objects, during decommission.

This option is only valid when `cleanupMetadata: true`.

When `cleanupData: false`, the operator may still delete Kubernetes PVC objects, but AIS is not asked to remove object data from the underlying disks.
Whether the data remains available to a future deployment depends on the PV type, reclaim policy, and storage provisioner.

When `cleanupData: true`, AIS is asked to remove user data during decommission.
Use this only when the cluster data (buckets and objects) should be deleted.

## PVC and PV Cleanup

During decommission with `cleanupMetadata: true`, the operator deletes AIS PVCs.
Deleting a PVC does not always delete the backing storage.

For static local PVs or PVs backed by `hostPath`, the PV often uses a `Retain` reclaim policy.
In that case, deleting the PVC can leave the PV object in `Released` state while the data remains on disk.
A future deployment may not bind to that PV until the PV object is deleted/recreated or otherwise made available again.

For dynamically provisioned PVs, behavior depends on the storage class reclaim policy and provisioner.
Some provisioners delete the backing volume when the PVC is deleted.
Others leave the data behind.

Before redeploying, check the PVs that were bound to AIS PVCs:

```console
kubectl get pv
kubectl get pvc -n <cluster-namespace>
```

If retained PVs are left in `Released` state, remove or recreate the PV objects according to your storage setup.
The provided scripts in [helm/ais/scripts](../helm/ais/scripts) can help automate this step.
For hostPath-backed PVs, removing the PV object does not remove the files from the host path.

## Changing Protocol

Changing an existing AIS cluster between HTTP and HTTPS requires fresh AIS state.
AIS state caches cluster maps and daemon URLs that include the old protocol.

Recommended flow:

1. Update the existing AIS custom resource so deletion uses:

   ```yaml
   cleanupMetadata: true
   cleanupData: false
   ```

2. Delete the AIS custom resource and wait for the operator to finish cleanup.
3. Check for remaining PVCs and retained PVs as described above.
4. Redeploy the AIS cluster with the new protocol and TLS settings. For Helm deployments, set the protocol and TLS options according to the [AIS Helm HTTPS deployment guide](../helm/ais/README.md#https-deployment).
