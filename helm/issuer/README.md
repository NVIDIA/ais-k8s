# Issuer Chart

This chart bootstraps a cluster-local certificate authority (CA) with [cert-manager](https://cert-manager.io/docs/), so AIS components can issue TLS certificates that chain to a common root.

It creates three cert-manager resources (see [`issuer-chart/templates/ca.yaml`](./issuer-chart/templates/ca.yaml)):

- A self-signed `Issuer` (`selfsigned-issuer`) that signs the CA certificate.
- A self-signed CA `Certificate` (`selfsigned-cert`, common name `selfsigned-ca`) whose key and certificate are stored in the `ca-root-secret` Secret.
- A CA issuer (`ca-issuer`) that signs leaf certificates from `ca-root-secret`.

This follows cert-manager's bootstrapping pattern: a [SelfSigned issuer](https://cert-manager.io/docs/configuration/selfsigned/) signs the CA certificate, and a [CA issuer](https://cert-manager.io/docs/configuration/ca/) issues leaf certificates from it.

Point a component's `issuerRef` at `ca-issuer` to have its certificate chain to this root.

## Scope

`issuerKind` selects the scope:

- `ClusterIssuer` (default): Cluster-wide, with the CA certificate and `ca-root-secret` in the `cert-manager` namespace.
- `Issuer`: Namespaced to the release namespace.

## Configuration

Set per-environment values under [`config/`](./config) and deploy with the [helmfile](./helmfile.yaml).
See [`issuer-chart/values.yaml`](./issuer-chart/values.yaml) for the full list.
Common values:

- `caCertificate.secretName` (default `ca-root-secret`): Secret holding the CA certificate and key.
- `caCertificate.duration` and `caCertificate.renewBefore`: CA lifetime and renewal window.
- `caCertificate.subject`: Set an organization or country to avoid an empty issuer DN.

For background on issuers, CA certificates, and trust, refer to the [cert-manager documentation](https://cert-manager.io/docs/usage/certificate/).
