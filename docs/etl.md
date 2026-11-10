# ETL Access

AIS-managed [ETL](https://github.com/NVIDIA/aistore/blob/main/docs/etl.md) runs each transformation in pods and services that the AIS targets create through the Kubernetes API.
The targets authenticate with the token of the cluster ServiceAccount, `<cluster-name>-sa`.
`spec.etl.enabled` controls whether the operator extends the ServiceAccount permissions to what ETL needs.
It defaults to `false`.
Setting it to `true` requires the `use` verb on `aistores/privileged`, because ETL access is [privileged](./privileged_access.md).

```yaml
spec:
  etl:
    enabled: true
```

Changing it on an existing cluster does not change the pod spec, so it does not restart any pod.
The operator updates the Role rules on the next reconcile.

## Permissions

The operator creates the `<cluster-name>-role` Role and binds it to the ServiceAccount in the cluster namespace.
Without ETL access, the Role only allows reads:

| Resource           | Verbs |
|--------------------|-------|
| `pods`, `services` | `get` |

AIS pods need these reads at startup.
The init container reads the LoadBalancer Service, and `aisnode` before v5.0 reads its own pod.

With ETL access, the Role allows:

| Resource           | Verbs                                                |
|--------------------|------------------------------------------------------|
| `pods`, `services` | `get`, `list`, `watch`, `create`, `update`, `delete` |
| `pods/log`         | `get`                                                |

AIS-managed ETL needs all of these permissions.
The targets create the ETL pod and service, watch the pod, read its logs, and delete both when the ETL stops.

## Risk

ETL access lets any process that has the pod token create pods in the cluster namespace.
A user who can edit the `AIStore` resource can set a custom container image, run it with the token, and create a pod directly.
This bypasses the [privileged access](./privileged_access.md) check, which only inspects the pods that the `AIStore` spec describes.

For this reason, the webhook treats ETL access as privileged.
While `spec.etl.enabled` is `true`, spec changes need the `use` verb on `aistores/privileged`.
Users without this access can still change specs that leave ETL access disabled.

To limit the pods that the token can create, enforce [Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/) on the namespace.
For rules that match specific users or ServiceAccounts, use a [ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/).
