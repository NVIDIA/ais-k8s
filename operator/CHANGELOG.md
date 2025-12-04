# Changelog

All notable changes to the AIStore K8s Operator project are documented in this file, starting with version v2.2.0.

Note: Changes to Helm charts, Ansible playbooks, and other deployment tools are not included.

We structure this changelog in accordance with [Keep a Changelog](https://keepachangelog.com/) guidelines, and this project follows [Semantic Versioning](https://semver.org/).

---
## v2.10.0

### Added

- Add support for OAuth compatible 3rd party auth services with password-based login. Set `auth.serviceURL` to the token login endpoint and configure `auth.usernamePassword.loginConf.clientID`
- Add `publicNetDNSMode` option. Supports `IP`, `Node`, or `Pod` values to determine what AIS uses for public network DNS. `IP` is the current default and matches existing deployments. `Pod` can be used with host networking to use pod DNS to resolve the host IP, allowing for more granular TLS certificates.
- Add `HOST_IPS` env var to init containers with field ref `status.hostIPs`. Used for future init containers to use to determine public host based on other options without an explicit variable from the operator.

### Changed

- Deprecate `enableNodeNameHost` added in v2.9.1 in favor of `publicNetDNSMode == Node`

## v2.9.3

### Changed

- Updated dependencies including AIS to latest `v4.1-rc1` commit to include latest `cluster_key` config

## v2.9.2

### Changed

- Avoid checking for removed env var `AIS_PUBLIC_HOSTNAME` for AIS container that would cause rollout on upgrade

## v2.9.1

### Added

- Add `enableNodeNameHost` to allow using K8s node hostnames for public interface. Uses K8s environment `spec.nodeName` instead of `status.hostIP` if enabled.

### Changed

- Fixed a bug where an empty `net.http.client_auth_tls` in AIS spec would cause an exception if TLS enabled

## v2.9.0

### Added

- Auth
  - Support for new AIS config options under `configToUpdate.auth.cluster_key` for configuring target validation of proxy-signed requests
  - TLS support for operator-to-auth service communication with `spec.auth.tls.caCertPath` configuration 
  - Fallback to default CA bundle path (`/etc/ssl/certs/auth-ca/ca.crt`) when `spec.auth.tls.caCertPath` is not configured
  - TLS config caching (6 hour TTL) to minimize disk I/O when loading CA certificates
  - `truststore` package for CA certificate loading and TLS configuration management
  - TLS certificate verification for the auth service can be disabled via `spec.auth.tls.insecureSkipVerify` (not recommended for production)
  - Operator mounts `ais-operator-auth-ca` ConfigMap to `/etc/ssl/certs/auth-ca` for Auth CA certificates when `authCAConfigmapName` is specified in the helm chart
  - OIDC issuer CA configuration via `spec.issuerCAConfigMap` for automatic certificate mounting and `auth.oidc.issuer_ca_bundle` configuration

- Autoscaling cluster size can now be limited by `spec.proxySpec.autoScale.sizeLimit` and `spec.targetSpec.autoScale.sizeLimit`

### Changed

- Auth
  - TLS configuration only applied for HTTPS URLs; HTTP connections skip
  - Return errors on TLS failures instead of silently falling back to insecure connections
  - Operator uses required audiences from AIStore cluster's `spec.configToUpdate.auth.required_claims.aud` to requests tokens with matching audiences during token exchange
  - Configurable Helm values (`authCAConfigmapName` and `aisCAConfigmapName`) for auth service and AIStore custom CA bundle configmaps
- Fixed a bug where resuming from shutdown state would become stuck on target scale up due to failing API calls
- Build: `mockgen` now installed to `LOCALBIN` with versioned suffix to prevent version mismatches that cause unnecessary diffs in generated mock files
- Use a common statefulset ready check for better enforcement of proxy rollout before starting target rollout
- Removed deprecation notice for `hostPathPrefix` option, with `stateStorageClass` still recommended for easier host cleanup

### Deprecated

- Defining the location of the admin credentials secret via `AIS_AUTHN_CM` ConfigMap
  - Use `spec.auth.usernamePassword.secretName` and `spec.auth.usernamePassword.secretNamespace` for static secrets
  - Use `spec.auth.tokenExchange` options for token exchange

## v2.8.0

### Added

- Helm chart: AIStore CRD now includes `helm.sh/resource-policy: keep` annotation to prevent CRD deletion during `helm uninstall`, protecting AIStore clusters from cascade deletion
- Add `clusterID` field in AIStore status to track the unique identifier for the cluster
- Operator to check cluster map proxy/target counts match the spec before setting the CR `Ready` condition to `True`
- Auth
  - RFC 8693 OAuth 2.0 Token Exchange support for AuthN token exchange with proper form-encoded requests
  - `TokenInfo` struct to track both token and expiration time returned from AuthN
  - Support for RFC 8693 standard response fields (`access_token`, `issued_token_type`, `token_type`, `expires_in`)
  - Backward compatibility with legacy `token` field in token exchange responses
  - AuthN configuration can now be specified directly in the AIStore CRD via `spec.auth` field with support for multiple authentication methods (username/password and token exchange) using CEL validation

### Changed

- Updated Go version to 1.25 and updated direct dependencies
- Reduced requeues and set specific requeue delays instead of exponential backoff
- Set `publishNotReadyAddresses: true` on headless SVCs for proxies and targets to enable pre-readiness peer discovery

- Auth
  - Token exchange now implements RFC 8693 specification with `application/x-www-form-urlencoded` content type
  - AuthN client methods now return `*TokenInfo` instead of plain string to include expiration metadata
  - Token exchange requests use RFC 8693 required fields: `grant_type`, `subject_token`, and `subject_token_type`
  - Operator AuthN configuration prioritizes CRD `spec.auth` over ConfigMap (ConfigMap approach is deprecated but supported for backward compatibility)
  - TLS configuration is now only applied for HTTPS URLs (HTTP connections no longer attempt TLS setup)
  - Updated CRD `configToUpdate.auth` options to match latest changes to AIS config including RSA key support, required audience claims, and OIDC issuer lookup

## v2.7.0

### Added

- Auto-scaling mode: Set `size: -1` to automatically scale proxy/target pods based on node selectors and tolerations
- Host path mounting: Use `useHostPath: true` in mount spec to bypass PV/PVC provisioning for direct host storage
- Auto-scale status tracking: New `AutoScaleStatus` field in cluster status tracks expected nodes for autoScaling clusters
- Token exchange authentication support to allow operators to exchange tokens (e.g., Kubernetes service account tokens or OIDC tokens) with authentication services for AIS JWT tokens
- Token expiration tracking: Support for OAuth `expires_in` field in token exchange responses with automatic token refresh
- Efficient token refresh: In-place token updates without rebuilding HTTP clients when tokens expire

### Changed

- Size validation: Allow `size: -1` for autoScaling mode
- Update target StatefulSet update strategy changed from RollingUpdate to OnDelete.
- Set maintenance mode before pod deletion during target rollouts.
- Reverse target rollout order to start from lowest ordinal (0 to N-1).

##  v2.6.0

### Added

- Read authN configuration and secret location from ConfigMap defined by `AIS_AUTHN_CM`

### Changed

- Fix cleanup job loop to skip deleted jobs and avoid unnecessary requeues during cluster cleanup.
- Enforce primary proxy reassignment, if required, during proxy scaledown (was previously best-effort).
- Refactor proxy scaledown handling to improve primary proxy reassignment and node decommissioning with better logging.

### Removed

- All AuthN environment variables from operator deployment
  - `AIS_AUTHN_SU_NAME`
  - `AIS_AUTHN_SU_PASS`
  - `AIS_AUTHN_SERVICE_HOST`
  - `AIS_AUTHN_SERVICE_PORT`
  - `AIS_AUTHN_USE_HTTPS`
##  v2.5.0

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