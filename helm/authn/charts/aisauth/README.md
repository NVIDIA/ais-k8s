# aisauth

Renders the AuthN resources the AIS operator cannot own:

- the `AIStoreAuth` custom resource
- a hostPath `PersistentVolume` (optional)

Credential and signing `Secret`s are not created by this chart. `adminSecret`, `hmacSecret` and `rsaPassphraseSecret` reference Secrets that already exist.

Everything else (ConfigMap, PVC, Deployment, Services, Certificate) is reconciled by the operator from the CR.

## Install

Install the AIS operator first. Then, run the commands below from `helm/authn`.

If Helm must own the credentials of AuthN, install the [Secrets](#secrets) first:

```console
export AUTHN_ADMIN_PASSWORD=...
helmfile -f charts/aisauth-secrets/helmfile.yaml sync
```

Then, deploy AuthN:

```console
helmfile sync -e local
```

The file `helm/authn/config/aisauth/<environment>.yaml` holds the values for the environment.

## Values

`adminSecret`, `hmacSecret`, `rsaPassphraseSecret`, `config`, `tls`, `persistence`, `externalAccess` and `deployment` are passed through to `AIStoreAuthSpec` unchanged.
For full spec options, see `kubectl explain aistoreauth.spec`, the [CRD](../../../../operator/config/base/crd/auth.ais.nvidia.com_aistoreauths.yaml), or the annotated [sample CR](../../../../operator/config/samples/ais_v1alpha1_aistoreauth.yaml).

The remaining values belong to the chart:

| Value                            | Description                                                                      |
| -------------------------------- | ---------------------------------------------------------------------------------- |
| `persistentVolume.hostPath`      | Host directory backing a local PV. The PV is rendered only when this is set.       |
| `persistentVolume.reclaimPolicy` | Reclaim policy for that PV. Defaults to `Retain`.                                  |
| `persistentVolume.nodeAffinity`  | Node affinity pinning the local PV to its host.                                    |

## Secrets

AuthN needs superuser credentials when it initializes a new database, and an HMAC key if you sign tokens with HMAC.
Reusing a retained volume that already holds a database needs neither.

This chart does not create these Secrets. Create them manually or have a controller create them (including an annotation/sidecar injector), then set `adminSecret.name`, `hmacSecret.name` or `rsaPassphraseSecret.name` to point at them (`SU-NAME` / `SU-PASS` for admin credentials, `SIGNING-KEY` for HMAC, `RSA-PASSPHRASE` for the RSA key passphrase).

The [`aisauth-secrets`](../aisauth-secrets) chart renders these Secrets. Install it before this chart: the `AIStoreAuth` validating webhook looks up every referenced Secret at admission and rejects the resource when one is missing.

See [AIStore AuthN docs](https://docs.nvidia.com/aistore/authn) for more information.

## TLS

`tls` has no chart default. With it unset the operator serves AuthN over plain HTTP, so superuser credentials and issued tokens cross the network unencrypted.

Either point at a `kubernetes.io/tls` Secret you manage:

```yaml
tls:
  secretName: ais-authn-tls
```

or have the operator provision one through cert-manager:

```yaml
tls:
  certificate:
    issuerRef:
      name: ais-ca-issuer
```

## Storage

`persistence` requires exactly one of `storageClass` or `volumeName`.

Dynamic provisioning:

```yaml
persistence:
  storageClass: openebs-hostpath
  size: 256Mi
```

If `storageClass` provisions local volumes, make sure it sets `volumeBindingMode: WaitForFirstConsumer` as WFC defers provisioning until the pod is scheduled, so the two always agree.

Local volume created by this chart:

```yaml
persistence:
  volumeName: ais-authn-pv
  size: 256Mi

persistentVolume:
  hostPath: /etc/ais/authn
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values: ["worker-1"]
```

Keep `persistence.deletionPolicy` and `persistentVolume.reclaimPolicy` consistent (both default to `Retain`).
With `persistence.deletionPolicy: Delete` the PVC is garbage collected along with the CR. A `Retain` PV then goes `Released` and stays there since Kubernetes will not rebind a volume whose `spec.claimRef` still names the deleted claim. The data is intact but recovering the volume requires clearing that reference by hand:

```console
kubectl patch pv <name> --type json -p '[{"op": "remove", "path": "/spec/claimRef"}]'
```
