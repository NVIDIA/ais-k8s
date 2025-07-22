# Changelog

All notable changes to the AIStore K8s Operator project are documented in this file, starting with version v2.2.0.

Note: Changes to Helm charts, Ansible playbooks, and other deployment tools are not included.

We structure this changelog in accordance with [Keep a Changelog](https://keepachangelog.com/) guidelines, and this project follows [Semantic Versioning](https://semver.org/).

---


## Unreleased

### Added

### Changed

- Allow configured cloud backends via `spec.configToUpdate.backend` in the absence of K8s secrets -- supports alternative secret injection
- Use statefulset status to simplify proxy rollout
- Update direct dependencies including AIS

## v2.4.0

### Added

- Add support for `labels` in `AIStore` CRD to allow users to specify custom labels that will be applied to either proxies or targets Pods.
- Add support for `AIS_TEST_API_MODE` environment variable to specify API mode for non-external LB clusters.
- Add support for `TEST_EPHEMERAL_CLUSTER` environment variable to skip cleanup/teardown when testing on ephemeral clusters (e.g. in CI).
- Add optional mount for `operator-tls` for supplying the operator with a certificate for client authentication.
- Add `ais-client-cert-path` for defining specific location of operator AIS client certificates.
- Add `ais-client-cert-per-cluster` to support separate certificate locations for each AIS cluster.
- Add `OPERATOR_SKIP_VERIFY_CRT` option to deployment, which will initially default to `True` to match previous deployments.
- Add TLS configuration to AIS API client, supporting additional CA trust and client Auth.
- Add kustomize patch to mount a configMap `ais-operator-ais-ca` for trusting specific AIS CAs.


### Changed

- Apply `imagePullSecrets` for image pull authentication to service account instead of individual proxy/target pod specs.
- Update RBAC rule in AIS service account to remove access to secrets and configmaps.
- Update kustomize structure to support overlays with different patch options on top of a common base.
- Remove kustomize usage of deprecated 'vars'.

## v2.3.0

### Added

- Support for the following env vars for testing
  - AIS_TEST_NODE_IMAGE
  - AIS_TEST_PREV_NODE_IMAGE
  - AIS_TEST_INIT_IMAGE
  - AIS_TEST_PREV_INIT_IMAGE

### Changed

- **COMPATIBILITY CHANGE**: Removed creation of a ServiceAccount with ClusterRole. ClusterRoles for existing clusters will be deleted.
  - Since ClusterRole is not namespaced, this allowed a creation of a namespaced AIS custom resource to result in a service account with cluster-wide access,
    which could then be impersonated.
  - This operator version will ONLY support AIS versions v3.28 or later.
  - Older AIS versions will error when trying to use the removed ClusterRole.
  - Related AIS change: https://github.com/NVIDIA/aistore/commit/160626c8fa44fc43ba7e9d42561dfbfe4216745e
