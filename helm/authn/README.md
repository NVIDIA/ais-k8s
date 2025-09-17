# AIS AuthN Helm Chart

>  **NOTE**: AuthN and its related Helm chart are under development. Breaking changes are to be expected, and it has NOT gone through a complete security audit.
Please review your deployment carefully and follow [our security policy](https://github.com/NVIDIA/ais-k8s/blob/main/SECURITY.md) to report any issues.

This directory contains a Helm chart and associated Helmfile for deploying the AIS AuthN service in K8s.

The exact resources deployed by this chart can be found in the [chart templates](./charts/authn/templates).

Values available to override are provided in the [chart values](./charts/authn/values.yaml.gotmpl) and [schema](./charts/authn/values.schema.json).

### Set up your environment config

We provide 3 environment types for deployment and reference. 

You can override the variables for these environments in the Helmfile command or create a new environment with its own config values template. 

Reference the [Helmfile](./helmfile.yaml) for configuring these values. 
Each environment can use a common environment file along with an additional [cert values file](./config/authn/cert) specific to their environment name (can be empty if not using TLS).

### TLS

Note the `tls` environment expects an existing certificate secret.

If the `createCert` value is set to true, a cert-manager certificate resource will be created that will output to this secret.

The `nvidia` and `oci` environments include values for configuring a certificate resource to create this certificate secret. 
Note the valid IP addresses and DNS names for the certificate must be provided as a value for these environments. 
To provide deployment-specific values, add the environment values to the [cert values file](./config/authn/cert)

### Required Env

The following environment variables MUST be provided at runtime to deploy:

- `AUTHN_ADMIN_PASSWORD`
- `JWT_SIGNING_KEY`

### Sync

Export the required values then run `helmfile sync` with your env: 

```console
helmfile sync -e default
```

### Removing a Deployment

Run `helmfile destroy` with your env:

```console
helmfile destroy -e default
```

