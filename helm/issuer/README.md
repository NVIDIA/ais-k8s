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

## CA key rotation

`caCertificate.privateKey.rotationPolicy` defaults to `Never`. We distribute the
self-signed CA's public certificate to clients, so renewal should keep the CA
signing key stable. Automatically replacing that key can break connections for
clients that still trust only the previous CA. This preserves cert-manager's
pre-v1.18 behavior; v1.18 changed its default to `Always`.

cert-manager renews certificates before expiry according to `renewBefore`. For
example, the chart default `renewBefore: 720h` schedules renewal 30 days early.
`Never` keeps the same signing key, so clients using the old, still-valid CA
certificate can continue trusting newly issued server certificates during that
window. It does not help after the old CA certificate expires: clients must receive
the renewed public CA certificate before then. Use `Always` only when you intend
to replace the signing key and have planned the client trust update.

With `Never`, changing the configured key algorithm or size does not replace an
existing key. cert-manager reports the mismatch and waits for intervention. Plan
a deliberate key rotation and client trust update before changing these settings.

## Configuration

Set per-environment values under [`config/`](./config) and deploy with the [helmfile](./helmfile.yaml).
An environment can set `valuesPath` to load values from ais-infra; otherwise the
Helmfile uses `./config/<environment>.yaml`.
See [`issuer-chart/values.yaml`](./issuer-chart/values.yaml) for the full list.
Common values:

- `caCertificate.secretName` (default `ca-root-secret`): Secret holding the CA certificate and key.
- `caCertificate.duration` and `caCertificate.renewBefore`: CA lifetime and renewal window.
- `caCertificate.privateKey.rotationPolicy`: Defaults to `Never`; explicitly set `Always` for planned key replacement.
- `caCertificate.subject`: Set an organization or country to avoid an empty issuer DN.

For background on issuers, CA certificates, and trust, refer to the [cert-manager documentation](https://cert-manager.io/docs/usage/certificate/).

## CA trust bundle distribution

Enable `trustBundle.enabled` to let Helm manage a trust-manager Bundle. It copies
only the public CA certificate from the CA Secret into a ConfigMap in each selected
namespace, and updates those ConfigMaps when the certificate changes. Consumers
such as the operator, admin client, and monitoring must mount and use the ConfigMap
for HTTPS verification. It does not update trust on external client machines.

Install trust-manager first and keep the CA Secret in its configured trust
namespace. Use `trustBundle.namespaceSelector` to select destination namespaces,
for example `matchLabels: {ais-trust: "true"}`. The CA private key is never copied.
This feature is disabled by default; standalone Bundles can still be managed separately.
