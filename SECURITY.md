# Security Policy: AIStore Kubernetes

For AIStore itself, see the [AIStore security policy](https://github.com/NVIDIA/aistore/blob/main/SECURITY.md).

## Support

Security updates and support are provided for the latest and most recent AIStore and AIS K8s Operator releases. Keep deployments on the latest stable release.

## Reporting a Vulnerability

Do not open a public issue or pull request. Report vulnerabilities privately using one of these methods:

1. **NVIDIA Vulnerability Disclosure Program (preferred):** Submit the [official NVIDIA vulnerability report](https://www.nvidia.com/en-us/security/report-vulnerability/).
2. **GitHub private vulnerability reporting:** Open this repository's **Security** tab and select **Report a vulnerability**.
3. **Email:** Send vulnerability details to [psirt@nvidia.com](mailto:psirt@nvidia.com) and copy [aistore@nvidia.com](mailto:aistore@nvidia.com) for AIStore-specific coordination.

Include the affected version or branch, vulnerability type, reproduction steps, proof of concept if available, and expected impact. NVIDIA PSIRT will acknowledge, assess, and coordinate remediation and disclosure.

## Security Architecture and Context

The AIS K8s Operator is the only supported component in this repository. Helm charts, Ansible playbooks, and other utilities are provided as unsupported deployment aids. The Operator's main security boundaries are the Kubernetes API, AIStore and AuthN service endpoints, Kubernetes Secrets, cluster storage, and worker hosts.

**Repository Exposure Classification:** Public. Basis: published in a public NVIDIA GitHub repository.

**Service Exposure Classification:** External / Regulated (high confidence). Basis: externally distributed production Kubernetes deployment tooling.

### Threat Model

1. **Unauthorized cluster changes:** The Operator reconciles `AIStore` and `AIStoreAuth` custom resources into workloads, services, storage, and RBAC resources. Access to create or update those resources must be restricted.
2. **Credential or data exposure:** TLS, AuthN, external services, and Secret references are deployment-configurable. Unsafe settings can expose credentials, management endpoints, or stored data.
3. **Privileged administrative actions:** The playbooks and `tools/state-manager` can change hosts, storage, and cluster state. Incorrect or untrusted inputs can damage or delete data.
4. **Untrusted deployment artifacts:** Container images, charts, manifests, and dependencies execute with cluster privileges and must come from trusted sources.

### Critical Security Assumptions

- Kubernetes authentication, RBAC, admission control, and Secret protection are correctly configured.
- Operators enable appropriate TLS, AuthN, and network restrictions for their environment.
- Kubernetes, the container runtime, worker hosts, and storage enforce their isolation and access controls.
- Administrative tools and playbooks are run only by trusted administrators against the intended cluster.
