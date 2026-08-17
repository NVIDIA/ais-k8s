# AIStoreAuthProfile

Operator release 3.3 introduces a new CRD type, `AIStoreAuthProfile`.
`AIStoreAuthProfile` is a cluster-scoped resource for configuring how clients may acquire valid access tokens for a given AIStore deployment.

## Motivation

The motivation for the separate type is primarily access control.
[K8s RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac) supports roles that can manage either all `AIStoreAuthProfile` resources or only specific ones.

Using `AIStoreAuthProfile` enables separating roles for editing profiles from the AIStore resource editor role.
This means users who can edit AIStore resources can no longer redirect the operator to send credentials to any arbitrary listener.

The custom resource also allows for the `use` verb, to separate admin-level management of the profiles and restrict client service accounts to the minimum necessary access.

## Creation

See [aistoreauthprofile.yaml](../operator/config/samples/aistoreauthprofile.yaml) for example manifests.

There are two mutually exclusive usage models:

- `usernamePassword`
  - Configure secret references and any additional login metadata
  - For OAuth 2.0 password-grant providers, set `loginConf.clientID` and `loginConf.endpoint` (the token endpoint path under `serviceURL`)
- `tokenExchange`
  - Configure the token exchange endpoint for the provided `serviceURL`

Both options share the common API configuration fields `spec.serviceURL` and `spec.tls`.

`spec.tls` allows trusting self-signed or private TLS certificate issuers:

```yaml
spec: 
  tls:
    caConfigMapRef:
      namespace: auth-config # ConfigMap with additional CA certificate for trust
      name: auth-provider-ca
      key: ca.crt # Specific key in the ConfigMap containing the PEM certificate
```

For development or trusted environments, `spec.tls.insecureSkipVerify` may also be enabled to skip certificate verification.

## RBAC

The operator comes bundled with three roles for managing `AIStoreAuthProfile` resources.
Helm installations prefix these with the operator chart's release name, which defaults to `ais-operator`:

- [aisauthprofile-editor-role](../operator/config/base/rbac-aisauth/aisauthprofile_editor_role.yaml)
- [aisauthprofile-user-role](../operator/config/base/rbac-aisauth/aisauthprofile_user_role.yaml)
- [aisauthprofile-viewer-role](../operator/config/base/rbac-aisauth/aisauthprofile_viewer_role.yaml)

To allow an account to `use` an existing `AIStoreAuthProfile`, simply bind the user role to an account with a [ClusterRoleBinding](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#rolebinding-and-clusterrolebinding).

If you want to control individual access to profiles, see [aisauthprofile_rbac.yaml](../operator/config/samples/aisauthprofile_rbac.yaml) for a sample restricted role.

## Usage

`AIStoreAuthProfile` can be referenced by any account with RBAC permissions and used by any client to determine auth server connection details.
Clients must still have access to any Secret or ConfigMap referenced in the profile.

In the `AIStore` custom resource spec, `spec.auth.profileRef` defines how the **AIS operator** obtains tokens from the auth provider. 
The tokens fetched using this profile information are included when the operator makes calls to this cluster's AIStore API for management.

```yaml
spec:
  auth:
    profileRef:
      name: aistore-auth-admin
```

The AIStore resource editor's `use` access will be checked by the validating webhook on submission.

See the local deployment with auth for example usage: 

- [AIStoreAuthProfile manifest](../local/manifests/auth-profile.yaml)
- [Helm values](../helm/ais/config/ais/local-auth.yaml)
