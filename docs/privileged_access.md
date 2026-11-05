# Privileged AIStore Access

## Motivation

Some `AIStore` spec fields give AIS pods privileges or access to the host node.
A user who can edit `AIStore` resources can run pods with elevated permissions, including host access, without explicit RBAC roles.
AIS still needs some of these settings, for example `hostPath` mounts, `hostNetwork`, and unsafe `sysctls` for network tuning.

The operator therefore keeps these fields available but restricts who can set them.
The `aistore-editor-role` does not grant this access.
To modify any AIStore spec that creates such pods, users must also have the `use` verb on the `aistores/privileged` virtual subresource.
Currently, the webhook only warns when a user lacks this access. A future release will reject the request.
The operator ships the `aistore-privileged-role` ClusterRole as an easy way to grant exactly this access.

## Restricted pods

The validating webhook builds the proxy and target pods that the spec describes.
It evaluates them against the [baseline Pod Security Standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/#baseline).
On every create and update where a pod violates the standard, the webhook checks that the user has `use` on `aistores/privileged`.
The warning and the operator log name the violations that the standard reports, for example `host namespaces (hostNetwork=true)`.

These fields always violate the standard when they are set (and not false):

- `spec.targetSpec.hostNetwork`
- `spec.targetSpec.mounts[*].useHostPath`
- `spec.stateStorage.hostPath`
- `spec.logsDir`

The `securityContext` and `aisContainerSecurityContext` fields violate the standard only for restricted values, such as `privileged: true` or an unsafe `sysctl`.
The operator pins the standard to Kubernetes v1.35, so a newer operator build does not change which pods violate it.

The `hostPort` fields are exempt, because a host port gives no access to the host beyond what the `aistore-editor-role` already grants.

The check applies to every spec change on such a cluster, not only to a change of these fields.
This stops an editor from combining a different image with the host access that the cluster already has.
Writes that leave the spec unchanged, such as a finalizer or annotation patch, do not require this access.

## RBAC

The webhook sends a `SubjectAccessReview` for the `use` verb on `aistores/privileged`, in the namespace and with the name of the `AIStore` resource.
Any RBAC rule that grants this access satisfies the check, for example a custom role or a wildcard rule such as the one in `cluster-admin`.

The operator ships the [aistore-privileged-role](../operator/config/base/rbac-ais/aistore_privileged_role.yaml) ClusterRole, which grants only this access.
Bind it in addition to the `aistore-editor-role`.

To allow a user to edit clusters with restricted pods in one namespace, bind the role with a RoleBinding:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ais-privileged-users
  namespace: ais
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ais-operator-aistore-privileged-role
subjects:
- kind: User
  name: alice@example.com
  apiGroup: rbac.authorization.k8s.io
```

Use a ClusterRoleBinding to allow the user in every namespace.
The ClusterRole name has the operator's name prefix, `ais-operator-` by default.

To limit a user to specific clusters, define a ClusterRole with `resourceNames` set to the `AIStore` names and bind it in the same way:

```yaml
rules:
- apiGroups:
  - ais.nvidia.com
  resources:
  - aistores/privileged
  resourceNames:
  - prod-ais
  verbs:
  - use
```

## Trust model

A user with `use` on `aistores/privileged` is trusted at the same level as a node administrator.
Grant this access only to cluster administrators and automation that deploys AIS clusters.
Grant it only for AIStore clusters that require an elevated security context or host access.
The operator's own pods do not need it. They meet the baseline standard by default.
