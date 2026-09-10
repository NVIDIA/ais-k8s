# AIS AuthN Helm Charts

> **NOTE**: AuthN and its related Helm charts are under development. Breaking changes are to be expected, and it has NOT gone through a complete security audit.
Please review your deployment carefully and follow [our security policy](https://github.com/NVIDIA/ais-k8s/blob/main/SECURITY.md) to report any issues.

This directory contains the Helm charts and associated Helmfile for deploying the AIS AuthN service in K8s.

The Helmfile deploys AuthN with one of two charts. Each environment enables one of them through its `release` value.

| Chart | Release | Condition | Deploys |
| ----- | ------- | --------- | ------- |
| [`authn`](./charts/authn)     | `ais-authn` | `release.authn.enabled`   | AuthN directly. |
| [`aisauth`](./charts/aisauth) | `aisauth`   | `release.aisauth.enabled` | An `AIStoreAuth` resource that the AIS operator reconciles. |

Each chart documents its own values and required environment in its README, [`authn`](./charts/authn/README.md) and [`aisauth`](./charts/aisauth/README.md).

### Set up your environment config

We provide 3 reference environments: `default`, `tls` and `local`.

You can override the variables for these environments in the Helmfile command or create a new environment with its own config values template.

Reference the [Helmfile](./helmfile.yaml) for configuring these values.
The `ais-authn` release reads the environment values file and the [cert values file](./config/authn/cert) set by `valuesPath` and `cert.valuesPath`. The `aisauth` release reads its environment file from [config/aisauth](./config/aisauth).

### Sync

Export the values the chart requires then run `helmfile sync` with your env:

```console
helmfile sync -e default
```

For the `aisauth` chart, install the AIS operator first. Then, install the Secrets. Then, install AuthN:

```console
helmfile -f charts/aisauth-secrets/helmfile.yaml sync
helmfile sync -e local
```

### Removing a Deployment

Run `helmfile destroy` with your env:

```console
helmfile destroy -e default
```

That removes the AuthN release only. With the `aisauth` chart the credential Secrets belong to a separate release:

```console
helmfile -f charts/aisauth-secrets/helmfile.yaml destroy
```
