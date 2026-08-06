# TLS / HTTPS

This guide describes deploying AIStore with TLS in Kubernetes.
It covers which components present certificates, how those certificates are issued, and how clients are told to trust them.

Refer to the [certificates diagram](./diagrams/certificates.jpg) for a visual of how the pieces fit together.

## What uses TLS

A fully TLS-enabled deployment involves several certificates.

- **AIS cluster:** Proxies and targets serve the AIS API over HTTPS with a server certificate.
- **AuthN:** The AuthN server serves over HTTPS with its own server certificate.
- **Clients (including the AIS K8s Operator):** Anything that connects to the cluster or AuthN must trust their server certificates.
For mutual TLS, a client also presents its own client certificate.

## Certificate sources

Certificates are issued through cert-manager, so a cert-manager `Issuer` or `ClusterIssuer` is always required.
Each chart that provisions a certificate accepts an `issuerRef`.

AIS components serve on in-cluster DNS names such as `ais-target-0.ais-target.ais.svc.cluster.local`.
The issuer must therefore be able to sign certificates for those `*.svc.cluster.local` names.
Most public and corporate CAs do not, so in practice this means you need a self-signed or private CA that does.
Because that CA is not in clients' system trust stores, clients must be given the CA certificate.
Refer to [Trusting a self-signed CA](#trusting-a-self-signed-ca).

## The cluster PKI

The repo ships an [`issuer`](../helm/issuer) chart that creates a self-signed root and exposes it as a `ClusterIssuer` named `ca-issuer`.
Point a component's `issuerRef` at `ca-issuer` to have its certificate chain to this common root.
Refer to the [issuer chart README](../helm/issuer/README.md) for its resources and configuration, and to the cert-manager [self-signed issuer](https://cert-manager.io/docs/configuration/selfsigned/) documentation it builds on.

## Trusting a self-signed CA

When a self-signed CA is used, each client needs the CA certificate to verify server certificates.
The certificate reaches a client as a ConfigMap holding `ca.crt`, mounted into the workload.
[trust-manager](https://cert-manager.io/docs/trust/trust-manager/) is the recommended way to distribute and sync that ConfigMap across namespaces.
Refer to its documentation to set it up for your deployment, and to the example [trust bundle](../local/manifests/trust-bundle.yaml) as a starting point.

The following clients need the CA:

- **AIS clients (apps, SDK, CLI):** Add the CA certificate to the client's trust store.
- **AIS admin client pod:** Point it at the CA ConfigMap with `spec.adminClient.caConfigMap`.
Refer to [Deploying an admin client](../operator/README.md#deploying-an-admin-client).
- **AIS K8s Operator:** Point it at the CA ConfigMap so it trusts the cluster and AuthN server certificates.
Refer to [Configure operator TLS and mTLS](../operator/README.md#configure-operator-tls-and-mtls).

> **Note:** Instead of trusting the CA, a client can skip certificate verification for non-production or casual access.
> Each client controls this on its own: `ais config cli set cluster.skip_verify_crt true` for the CLI, the `AIS_SKIP_VERIFY_CRT` environment variable for AIS admin client pods, `-k` for `curl`, and `spec.operatorSkipVerifyCrt: true` for the operator.
> Skipping verification disables TLS protection and exposes clients to man-in-the-middle attacks, so it is *not* recommended for production.

### Exporting the CA certificate

Clients outside the cluster need the CA certificate as a local file.
Every certificate secret issued by `ca-issuer` carries the root it chains to under `ca.crt`.

```console
kubectl get secret -n <namespace> <tls-secret> -o jsonpath='{.data.ca\.crt}' | base64 -d > ais_ca.crt
```

## AIS cluster

Proxies and targets serve the AIS API over HTTPS once TLS is enabled.
The operator path is described below.
For Helm, set `protocol: https` and a `tls` block in your AIS values file, and refer to the [AIS Helm HTTPS deployment](../helm/ais/README.md#https-deployment).

**Important:** Before proceeding, ensure `cert-manager` (or equivalent) is installed.

Deploying with HTTPS through the operator requires two spec entries:

1. **`spec.tls`:** Tells the operator how to provision and mount the certificate (secret reference or cert-manager Certificate).
2. **`spec.configToUpdate.net.http.use_https: true`:** Tells AIS to serve HTTPS.

```yaml
spec:
  tls:
    certificate:
      issuerRef:
        name: ca-issuer
        kind: ClusterIssuer
    # Or, to reference an existing Kubernetes TLS secret,
    # use secretName: tls-certs
  configToUpdate:
    net:
      http:
        use_https: true
        skip_verify: false  # Set true only when using self-signed certs without a trusted CA
```

> **Note:** Our Helm charts populate the `configToUpdate.net.http` HTTPS fields (`use_https`, `skip_verify`) automatically when `spec.tls` is configured.

There are three ways to supply the certificate.

### Using a secret mount

If you bring your own Kubernetes Secret containing the certificate and key, define it with `spec.tls.secretName`.
The secret must contain keys `tls.crt` and `tls.key` (standard `kubernetes.io/tls` layout); for mTLS, also include `ca.crt`.
AIStore pods mount the secret contents at `/var/certs`, and the operator does not manage the certificate's lifecycle or SANs in this mode.

To create this secret with Helm, refer to the [AIS Helm HTTPS deployment](../helm/ais/README.md#https-deployment).

### Using an operator-managed certificate

With `spec.tls.certificate.mode: secret` (the default), the operator creates a cert-manager `Certificate` resource, and every AIS pod mounts the issued Secret.
The certificate's SAN list reflects the nodes that may host AIS pods, following `publicNetDNSMode`:

- `Node`: SANs include the names of nodes matching either the target or proxy `nodeSelector` (and tolerating their `tolerations`).
- `IP`: SANs include those nodes' primary IPs (InternalIP, falling back to ExternalIP).
- `Pod`: Pod-scoped DNS only, so node identities are not included.

When autoscaling is enabled, the SAN list tracks `Status.AutoScaleStatus.ExpectedTargetNodes` and `ExpectedProxyNodes`, with the same `publicNetDNSMode` semantics.

If the proxy or target `nodeSelector` is unset, every node in the cluster matches and ends up in the certificate.
Set explicit selectors so the certificate reflects only the nodes that may host AIS pods.

### Using the CSI driver

With the [cert-manager CSI driver](https://github.com/cert-manager/csi-driver) installed, the driver issues a fresh certificate per pod at volume mount time, directly from your issuer.
Refer to the [sample configuration](../operator/config/samples/ais_v1beta1_aistore_tls_certmanager_csi.yaml) for RBAC and an Issuer for use with Vault.

Node-derived SANs are not auto-included in CSI mode, so use `spec.tls.certificate.additionalDNSNames` or `spec.hostnameMap` to pin extra hostnames or IPs.

## AuthN

Enable HTTPS on AuthN with `tls.enabled: true`.
With `tls.createCert: true`, the chart creates a cert-manager `Certificate` from the configured `issuerRef` and DNS names:

```yaml
tls:
  enabled: true
  createCert: true
  certificate:
    issuerRef:
      name: ca-issuer
      kind: ClusterIssuer
    dnsNames:
      - "ais-authn.ais.svc.cluster.local"
      - "<external-hostname>"
```

To use an existing certificate instead, set `createCert: false` and point `tls.secretName` at a `kubernetes.io/tls` Secret.

## Mutual TLS (mTLS)

Mutual TLS is optional, even on a TLS cluster.
With it, the operator presents its own client certificate to AIS, and AIS verifies it against the cluster CA.
The operator's client certificate must therefore come from a CA included in the cluster's `ca.crt`.
The simplest setup issues both the cluster certificate and the operator client certificate from the same issuer, such as `ca-issuer`.
Refer to [Configure operator TLS and mTLS](../operator/README.md#configure-operator-tls-and-mtls) for the client certificate setup.
