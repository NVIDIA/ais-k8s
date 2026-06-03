## AIS Helm Chart and Helmfile

This file contains instructions for the provided [helmfile](./helmfile.yaml) and the included [AIS Helm Chart](./charts/ais-cluster/Chart.yaml). 

For details on the values accepted by the AIS chart, see the [values schema](./charts/ais-cluster/values.schema.json). 

We use helmfile to manage values files for different deployments as well as to automate running scripts for various administrative purposes.
See the [cluster management section](#cluster-management) before enabling any of the additional values in the helmfile. 

## Cluster Management

### Node Labeling

The [label-nodes.sh](./scripts/label-nodes.sh) convenience script labels nodes with `nvidia.com/ais-proxy=<cluster>` and `nvidia.com/ais-target=<cluster>`.
These labels are used for scheduling via `nodeSelector` and by the `ais-create-pv` chart to discover target nodes.

```bash
./scripts/label-nodes.sh <cluster> <node1,node2,...|--all>
``` 

### PV Creation

The provided helmfile includes the [ais-create-pv](./charts/create-pv/) release, enabled by setting `createPV.enabled: true` for the environment.
This chart queries for labeled target nodes and creates host path PVs for each mount-path on every labeled target.
See [Target Data Persistent Volumes](../../docs/storage_volumes.md) for details on volume mounts.

If you want to use an existing set of PVs, set `createPV.enabled: false`.
You can also change the `storageClass` option to instruct AIS target pods to mount a different existing storage class.

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
| [ais-create-pv](./charts/create-pv/)         | Create HostPath PersistentVolumes for labeled target nodes                            |
| [tls-cert](./charts/tls-cert/)               | Create a cert-manager certificate                                                     |
                                                          
