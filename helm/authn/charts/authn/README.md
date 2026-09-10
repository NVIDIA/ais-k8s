# authn

Deploys AuthN directly. The chart creates the Deployment, the Services, the ConfigMap, the PV, the PVC and the credential Secrets.

The exact resources deployed by this chart can be found in the [chart templates](./templates).

Values available to override are provided in the [chart values](./values.yaml.gotmpl) and [schema](./values.schema.json).

## Required Env

The following environment variables MUST be provided at runtime to deploy:

- `AUTHN_ADMIN_PASSWORD`
- `JWT_SIGNING_KEY`

## TLS

See [docs/tls.md](../../../../docs/tls.md) for the TLS overview and how AuthN fits in. In this chart, set `tls.enabled: true` to serve HTTPS. With `tls.createCert: true` a cert-manager `Certificate` is created from the `issuerRef` and DNS names in the [cert values file](../../config/authn/cert); otherwise point `tls.secretName` at an existing certificate secret.
