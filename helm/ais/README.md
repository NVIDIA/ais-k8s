## AIS Helm Chart and Helmfile

This file contains instructions for the provided [helmfile](./helmfile.yaml) and the included [AIS Helm Chart](./charts/ais-cluster/Chart.yaml). 

For details on the values accepted by the AIS chart, see the [values schema](./charts/ais-cluster/values.schema.json). 

We use helmfile to manage values files for different deployments as well as to automate running scripts for various administrative purposes.
See the [cluster management section](#cluster-management) before enabling any of the additional values in the helmfile. 

## Cluster Management

### Node Labeling

The [label-nodes.sh](./scripts/label-nodes.sh) convenience script labels nodes with `nvidia.com/ais-proxy=<cluster>` and `nvidia.com/ais-target=<cluster>`.
These labels are used for scheduling via `nodeSelector` and by `create-pvs.sh` to discover target nodes.

```bash
./scripts/label-nodes.sh <cluster> <node1,node2,...|--all>
``` 

### PV Creation

The provided helmfile will run the [create-pvs.sh](./scripts/create-pvs.sh) if the value is set for the environment: `createPV.enabled: true`.
This queries for labeled target nodes and creates HostPath persistent volumes for each mount-path on every labeled target.

If you want to use an existing set of PVs, set `createPV.enabled: false`.
You can also change the `storageClass` option to instruct AIS target pods to mount a different existing storage class.

## HTTPS deployment

To deploy with TLS enabled, simply set the following values in your **AIS** chart values file

```
protocol: https
https:
    skipVerifyCert: false // optional
    tlsSecret: "tls-certs" // Required only if using secret mount. Mounts to /var/certs
```

This will update the AIS config and mount the secret if provided (read below for creating).

### Generating the certificates

To use a self-signed ClusterIssuer, follow the [README](../README.md#install-cluster-issuer--optional) to install the [cluster-issuer chart](../cluster-issuer).

If you want to use the [tls-cert chart](./charts/tls-cert) to actually generate and manage the certificates, set the value `https.enabled: true` for your environment in the [helmfile](./helmfile.yaml).

Create a values file for your environment in [config/tls-cert](./config/tls-cert).

Finally, update the values file including the reference to the Issuer or ClusterIssuer you want to use. 

## Cloud Credentials

To configure backend provider secrets managed by helm, set the value `cloudSecrets.enabled: true` for your environment in the [helmfile](./helmfile.yaml). 

Then, add a configuration values file in the [config/cloud](./config/cloud/) directory to populate the variables used by the [cloud-secrets templates](./charts/cloud-secrets/templates/).

Add references to the local files you want to use. Example for sjc11 (be sure to update your paths correctly):
  ```yaml
  aws_config: |-
  {{ readFile (printf "%s/.aws/sjc11/config" (env "HOME")) | indent 2 }}

  aws_credentials: |-
  {{ readFile (printf "%s/.aws/sjc11/credentials" (env "HOME")) | indent 2 }}

  gcp_json: |-
  {{ readFile (printf "%s/.gcp/sjc11/gcp.json" (env "HOME")) | indent 2 }}
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
| [tls-cert](./charts/tls-cert/)               | Create a cert-manager certificate                                                     |
                                                          
