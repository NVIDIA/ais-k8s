# aisauth-secrets

Renders the credential and signing `Secret`s AuthN needs:

- an admin (superuser) credentials `Secret`
- an HMAC signing key `Secret` (rendered only when `hmacKey` is set)
- an RSA passphrase `Secret` (rendered only when `rsaPassphrase` is set)

Point the `aisauth` chart's `adminSecret.name`, `hmacSecret.name` and `rsaPassphraseSecret.name` at the names this chart renders.

Install it when Helm should own AuthN's credentials. `adminPassword` is required: creating the admin `Secret` is the reason this chart exists.

## Install order

Use [`helmfile.yaml`](./helmfile.yaml) to install this chart before the `aisauth` chart. The `AIStoreAuth` webhook resolves every referenced `Secret` at admission and rejects the resource when one is missing.

## Values

| Value                     | Description                                                                      |
| ------------------------- | -------------------------------------------------------------------------------- |
| `adminPassword`           | Superuser password. Required.                                                    |
| `adminUsername`           | Superuser name. Defaults to `admin`.                                             |
| `hmacKey`                 | HMAC signing key. Renders a Secret.                                              |
| `rsaPassphrase`           | Passphrase protecting the RSA private key. Renders a Secret.                     |
| `adminSecretName`         | Rename the admin Secret. Defaults to `ais-authn-su-creds`.                       |
| `hmacSecretName`          | Rename the HMAC Secret. Defaults to `ais-authn-jwt-signing-key`.                 |
| `rsaPassphraseSecretName` | Rename the RSA passphrase Secret. Defaults to `ais-authn-rsa-passphrase`.        |

The `*SecretName` values rename `Secret`s this chart owns. Never point one at a `Secret` owned by anything else: Helm fails the install on an ownership conflict rather than adopting it.

## Usage

Secret material is read from the environment through [`values.yaml.gotmpl`](./values.yaml.gotmpl), so it never has to be written into a values file:

```console
export AUTHN_ADMIN_PASSWORD=...   # required
export JWT_SIGNING_KEY=...        # optional, renders the HMAC Secret
export AUTHN_RSA_PASSPHRASE=...   # optional, renders the RSA passphrase Secret
```

Then, run the helmfile of this chart from `helm/authn`.

```console
helmfile -f charts/aisauth-secrets/helmfile.yaml sync
```

With only `AUTHN_ADMIN_PASSWORD` set this renders a `Secret` named `ais-authn-su-creds` holding `SU-NAME` and `SU-PASS`. Reference it from the `aisauth` chart:

```yaml
adminSecret:
  name: ais-authn-su-creds
```

## Signing keys

Setting `hmacKey` only renders the Secret. HMAC signing is selected by referencing it as the `aisauth` chart's `hmacSecret.name`. With that reference absent, AuthN stays on RSA.

When AuthN signs with HMAC, the AIStore cluster verifies those tokens with the same key: set the AIStore resource's `spec.authNSecretName` to the HMAC Secret name, in the AIStore namespace. See [docs/authentication.md](../../../../docs/authentication.md).

See [AIStore AuthN docs](https://docs.nvidia.com/aistore/authn) for more information.
