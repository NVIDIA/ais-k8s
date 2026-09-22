# AIStore Authentication Server (AuthN) in Kubernetes

>  **NOTE**: AuthN and its related deployment automations are under development. Breaking changes are to be expected, and it has NOT gone through a complete security audit.
Please review your deployment carefully and follow [our security policy](https://github.com/NVIDIA/ais-k8s/blob/main/SECURITY.md) to report any issues.

The AIStore Authentication Server (AuthN) provides secure access to AIStore by leveraging [OAuth 2.0](https://oauth.net/2/) compliant [JSON Web Tokens (JWT)](https://datatracker.ietf.org/doc/html/rfc7519). 

For more information on AuthN, visit the [AIStore AuthN documentation](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md).

## Deploying AuthN in Kubernetes

### Deploy with Helm

The best way to deploy AuthN is to use our [provided Helm charts](../helm/authn/README.md).

There are two deployment models, and each Helm chart covers one of them:

| Model | Chart | Who creates the resources |
| ----- | ----- | ------------------------- |
| Operator managed | [`aisauth`](../helm/authn/charts/aisauth/README.md) | The chart creates an `AIStoreAuth` custom resource. The operator reconciles it into the ConfigMap, PVC, Deployment, Services, and Certificate. |
| Chart managed | [`authn`](../helm/authn/charts/authn/README.md) | The chart creates every resource directly. |

The rest of this section covers the operator-managed model.

### AIStoreAuth Custom Resource

`AIStoreAuth` is a namespaced resource in API group `auth.ais.nvidia.com/v1alpha1`, with the short name `aisauth`.

For the spec fields, run `kubectl explain aistoreauth.spec`, or read the annotated [sample resource](../operator/config/samples/ais_v1alpha1_aistoreauth.yaml).
For the chart values that map onto the spec, see the [`aisauth` chart README](../helm/authn/charts/aisauth/README.md).

Run `kubectl describe` to check the state of a deployment through the `Ready` condition:

```console
kubectl get aistoreauth -n ais -o wide
kubectl describe aistoreauth -n ais <name>
```

### Operator Resources

The operator derives every name from the name of the `AIStoreAuth` resource. 
For a resource named `ais-authn`:

| Resource | Name | Purpose |
|----------|------|---------|
| Deployment | `ais-authn` | Runs the AuthN pod. |
| ConfigMap | `ais-authn-config` | Holds the rendered `authn.json`, mounted read-only. |
| PersistentVolumeClaim | `ais-authn-storage` | Holds the user database and the RSA keys. |
| Service (ClusterIP) | `ais-authn` | The in-cluster endpoint. Always created. |
| Service (NodePort) | `ais-authn-nodeport` | Created only when `spec.externalAccess.nodePort` is set. |
| Service (LoadBalancer) | `ais-authn-lb` | Created only when `spec.externalAccess.loadBalancer` is set. |
| Certificate | `ais-authn-authn-tls-cert` | Created only in `secret` TLS mode. Writes the Secret `ais-authn-authn-tls`. |

The operator publishes the in-cluster endpoint in `status.serviceURL`. 
Use that value for `spec.serviceURL` in the [AIStoreAuthProfile](./auth_profile.md) and for `AIS_AUTHN_URL` in clients.

The pod mounts the data volume at `/etc/ais/authn` and the TLS certificate at `/var/certs`.

The operator does not create the credential Secrets or the `PersistentVolume`. 
Create those first.

### Token Signing

The AuthN server signs tokens with RSA or with HMAC. 
`spec.hmacSecret` selects between them:

- **RSA (default).** Leave `spec.hmacSecret` unset. AuthN generates the key pair on the data volume. Set `spec.rsaPassphraseSecret` to protect the private key with a passphrase.
- **HMAC.** Set `spec.hmacSecret`. AuthN signs with the shared key that Secret holds.

Each Secret maps to an environment variable on the AuthN container:

| Spec field | Secret key | Environment variable |
|------------|------------|----------------------|
| `spec.adminSecret` | `SU-NAME` | `AIS_AUTHN_SU_NAME` |
| `spec.adminSecret` | `SU-PASS` | `AIS_AUTHN_SU_PASS` |
| `spec.hmacSecret` | `SIGNING-KEY` | `AIS_AUTHN_SECRET_KEY` |
| `spec.rsaPassphraseSecret` | `RSA-PASSPHRASE` | `AIS_AUTHN_PRIVATE_KEY_PASS` |

### Wiring OIDC Discovery to AIStore

When the AIStore cluster looks up the issuer through OIDC, the proxies fetch the public key from the AuthN discovery endpoint.
Set `spec.config.net.externalURL` to the URL the proxies use to reach AuthN:

```yaml
spec:
  config:
    net:
      externalURL: https://ais-authn.ais.svc.cluster.local:52001
```

Then list that same URL in `spec.configToUpdate.auth.oidc.allowed_iss` on the AIStore resource.
The two values must match exactly.

If AuthN serves HTTPS with a private CA, the proxies must also trust that CA. 
Set `spec.issuerCAConfigMap` on the AIStore resource.

## AuthN Clients

See the [authentication docs](./authentication.md) for information about configuring the AIStore cluster and operator to use authN and other authentication services.

To interact with AIStore, clients need a signed JWT token.
By default, an `admin` user with super-user privileges is created with a mandatory provided password.
This password must be set through [environment variables](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#environment-and-configuration).
Admins can then create roles and assign users to those roles.
For a typical setup process, refer to the [Getting Started Guide](https://github.com/NVIDIA/aistore/blob/main/docs/authn.md#getting-started).

Set the following environment variable to point to the appropriate AuthN server to log in and obtain the token:

```bash
# For external clients
export AIS_AUTHN_URL=https://<NodePort-service-IP>:30001

# For internal clients
export AIS_AUTHN_URL=https://ais-authn.ais:52001
```

When deployed as an AIStoreAuth custom resource, read the in-cluster URL from the resource:

```console
kubectl get aistoreauth -n ais <name> -o jsonpath='{.status.serviceURL}'
```

## Switching Between HTTP and HTTPS (TLS) for the AuthN Server

For how AuthN certificates are issued and trusted, see the [TLS guide](./tls.md).

To switch the protocol of an existing AuthN server, apply the new configuration over the current deployment.
This redeploys the AuthN server with the updated settings.

When deployed as an AIStoreAuth custom resource, add or remove `spec.tls` in the `AIStoreAuth` spec.
See [AuthN TLS](./tls.md#authn) on how to configure it.
The operator reconciles the change and updates `status.serviceURL`.

Each protocol switch also needs two updates outside the `AIStoreAuth` resource:

1. Update `spec.serviceURL`, and if needed `spec.tls`, in the [AIStoreAuthProfile](./auth_profile.md) that the AIStore spec references.
2. If the AIStore cluster looks up the issuer through OIDC, update the issuer URLs to match. See [Wiring OIDC Discovery to AIStore](#wiring-oidc-discovery-to-aistore).
