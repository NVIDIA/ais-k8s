## AIS Helm Chart

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

To use a self-signed ClusterIssuer, follow the [README](../README.md#install-cluster-issuer-optional) to install the [cluster-issuer chart](../cluster-issuer).

If you want to use the [tls-cert chart](./charts/tls-cert) to actually generate the certificates, set the value `https.enabled: true` for your environment in the [helmfile](./helmfile.yaml).

Create a values file for your environment in [config/tls-cert](./config/tls-cert).

Finally, update the values file including the reference to the Issuer or ClusterIssuer you want to use. 

## Cloud Credentials

To configure backend provider secrets from the helm charts, set the value `cloud-secrets.enabled: true` for your environment in the [helmfile](./helmfile.yaml). 

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

For OCI, setting the `OCI_COMPARTMENT_OCID` variable is necessary to provide a default compartment.


## PV Creation

The AIS chart will run the [create-pvs.sh](./scripts/create-pvs.sh) if the value is set for the environment: `ais-create-pv.enabled: true`.
This will use helm to template and automatically create HostPath persistent volumes for each of the mount-paths for every target in the cluster.

If you want to use an existing set of PVs, set `ais-create-pv.enabled: false`.
You can also change the `storageClass` option to instruct AIS target pods to mount a different existing storage class.

## Running the deployment 

From the `ais` directory, run: 

```bash 
helmfile sync --environment <your-env>
```

To uninstall:
```bash
helmfile delete --environment <your-env>
```

| Chart                                                      | Description                                                                           |
|------------------------------------------------------------|---------------------------------------------------------------------------------------|
| [ais-cloud-secrets](./charts/cloud-secrets/) | Create k8s secrets from local files for cloud backends                                |
| [ais-cluster](./charts/ais-cluster/)         | Create an AIS cluster resource, with the expectation the operator is already deployed |
| [tls-cert](./charts/tls-cert/)               | Create a cert-manager certificate                                                     |
                                                          
