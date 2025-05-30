# Changelog

All notable changes to the AIStore K8s Operator project are documented in this file, starting with version v2.2.0.

Note: Changes to Helm charts, Ansible playbooks, and other deployment tools are not included. 

We structure this changelog in accordance with [Keep a Changelog](https://keepachangelog.com/) guidelines, and this project follows [Semantic Versioning](https://semver.org/).

---

## Unreleased

### Added

### Changed

- Update RBAC rule in AIS service account to remove access to secrets and configmaps.

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
