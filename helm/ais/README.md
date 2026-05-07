## AIS Helm Chart and Helmfile

This file contains instructions for the provided [helmfile](./helmfile.yaml) and the included [AIS Helm Chart](./charts/ais-cluster/Chart.yaml). 

For details on the values accepted by the AIS chart, see the [values schema](./charts/ais-cluster/values.schema.json). 

We use helmfile to manage values files for different deployments as well as to automate running scripts for various administrative purposes.
See the [cluster management section](#cluster-management) before enabling any of the additional values in the helmfile. 

## Cluster Management

### Node Labeling

The [label-nodes.sh](./scripts/label-nodes.sh) convenience script labels nodes with `nvidia.com/ais-proxy=<cluster>` and `nvidia.com/ais-target=<cluster>`.
These labels are used for scheduling via `nodeSelector` and by the `ais-create-target-pv` chart to discover target nodes.

```bash
./scripts/label-nodes.sh <cluster> <node1,node2,...|--all>
``` 

Deleting the AIS cluster leaves these labels in place, so a redeployment schedules onto the same nodes.
Pass `--remove` to strip both labels and release the nodes. With `--all`, this selects the nodes currently labeled for the cluster.

```bash
./scripts/label-nodes.sh --remove <cluster> <node1,node2,...|--all>
```

### PV Creation

The provided helmfile includes the [ais-create-target-pv](./charts/create-target-pv/) release, enabled by setting `createTargetPV.enabled: true` for the environment.
This chart templates one host path PV per mount-path on every labeled target node (discovered by label or a provided list) and makes them available under a `WaitForFirstConsumer` storage class.
Each PV binds once the operator creates a target PVC that the scheduler places on that node.
See [Target Data Persistent Volumes](../../docs/storage_volumes.md) for details on volume mounts.

To use an existing set of PVs, set `createTargetPV.enabled: false`.
The `storageClass` option instructs AIS target pods to mount a different existing storage class.

The older [ais-create-target-pv-job](./charts/create-target-pv-job/) chart provisions the target PVs through a pre-install Job.
Its PVs carry a `claimRef` naming the one target PVC allowed to bind, so every node has to be mapped to a target ordinal up front instead of the scheduler resolving it through `WaitForFirstConsumer`, and those PVs are not Helm-managed.

> **Note:** It is not possible to in-place upgrade from `create-target-pv-job` to the new `create-target-pv` chart. The old Job-created PVs are not Helm-owned and collide by name, so moving an existing cluster to this chart will require redeploying the cluster *entirely*.

### Scaling

The `size` value in your AIS values file sets the number of proxy and target pods.
`proxySpec.size` and `targetSpec.size` override it for that component, so a cluster can run a different number of proxies than targets.
Pods schedule only onto nodes carrying the cluster's labels, and `ais-create-target-pv` discovers targets the same way, so [label](#node-labeling) any new nodes before raising a size.
Then run `helmfile sync` again.

Lowering a size removes the highest-ordinal pods.
The operator decommissions them via the AIStore cluster API first, and `targetSpec.scaleDownMode` controls what happens to a removed target's data.

### Shutdown and Removal

`shutdownCluster`, `cleanupMetadata`, and `cleanupData` are read off the live custom resource, so set them in your values file and run `helmfile sync` before uninstalling.

For a temporary stop, set `shutdownCluster: true` and sync.
The operator scales the cluster to zero and leaves data and configuration intact.
Set it back to `false` and sync to bring the cluster back.

For permanent removal, set the cleanup options for the outcome you want, sync, then [uninstall](#running-the-deployment):

```yaml
cleanupMetadata: true  # decommission AIS and remove its state
cleanupData: true      # also delete buckets and objects, requires cleanupMetadata
```

See the [redeployment guide](../../docs/redeployment.md) for what each option does and for the PVs and host path files left behind afterward.

## HTTPS deployment

Enable TLS by setting `protocol: https` and a `tls` block in your AIS values file. To have cert-manager generate and manage the cert, enable the `https` release (`https.enabled: true`) in the [helmfile](./helmfile.yaml) with a [config/tls-cert](./config/tls-cert) values file. See the [TLS guide](../../docs/tls.md) for cert options and the self-signed-CA model.

## Cloud Credentials

To configure backend provider secrets managed by helm, set the value `cloudSecrets.enabled: true` for your environment in the [helmfile](./helmfile.yaml). 

Then, add a configuration values file in the [config/cloud](./config/cloud/) directory to populate the variables used by the [cloud-secrets templates](./charts/cloud-secrets/templates/).

Add references to the local files you want to use. See the following example (be sure to update paths if necessary):
  ```yaml
  aws_config: |-
  {{ readFile (printf "%s/.aws/config" (env "HOME")) | indent 2 }}

  aws_credentials: |-
  {{ readFile (printf "%s/.aws/credentials" (env "HOME")) | indent 2 }}

  gcp_json: |-
  {{ readFile (printf "%s/.gcp/gcp.json" (env "HOME")) | indent 2 }}
  ```


Note this chart only creates the secrets to be mounted by the targets.
Extra environment variables can be provided through the values for the main AIS chart.

For OCI, setting the `OCI_COMPARTMENT_OCID` environment variable is necessary to provide a default compartment.

## Running the deployment 

From the `ais` directory, run: 

```bash 
helmfile sync --environment <your-env>
```

To uninstall:
```bash
helmfile destroy --environment <your-env>
```

| Chart                                                      | Description                                                                           |
|------------------------------------------------------------|---------------------------------------------------------------------------------------|
| [ais-cloud-secrets](./charts/cloud-secrets/) | Create k8s secrets from local files for cloud backends                                |
| [ais-cluster](./charts/ais-cluster/)         | Create an AIS cluster resource, with the expectation the operator is already deployed |
| [ais-create-target-pv](./charts/create-target-pv/)         | Create HostPath PersistentVolumes for target nodes, bound via WaitForFirstConsumer    |
| [ais-create-target-pv-job](./charts/create-target-pv-job/) | Create claimRef-pinned target PVs via a pre-install Job, not Helm-managed              |
| [tls-cert](./charts/tls-cert/)               | Create a cert-manager certificate                                                     |
                                                          
