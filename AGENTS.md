# AGENTS.md

## Project Overview

This repository is the deployment toolkit for running [AIStore](https://github.com/NVIDIA/aistore) on Kubernetes. It is a monorepo: the **AIS K8s Operator** (`operator/`, Go) is the primary component. See `SECURITY.md` for the security boundary and threat model before making changes that touch RBAC, auth, or CI.

Key subprojects:

- `operator/` — the Go operator (Kubebuilder/operator-sdk). Reconciles the `AIStore` and `AIStoreAuth` CRDs into workloads, services, storage, and RBAC, and serves validating webhooks for both plus `AIStoreAuthProfile`, which `AIStoreAuth` references. See [operator/AGENTS.md](operator/AGENTS.md) for its layout, dev workflow, testing, and code style rules.
- `helm/` — Helm charts for the operator, an AIStore cluster (`ais`), the AuthN server, `aisloader`, and a cert-manager issuer.
- `playbooks/` — Ansible playbooks for node host configuration, cloud backend credentials, and OS hardening. Run them in the order given in `playbooks/README.md`.
- `auth/keycloak/` — sample local deployment of Keycloak, one alternative AuthN provider AIStore can be configured to use.
- `ais-operator-helper/` — Go image holding `cleanup-helper`, which the operator runs as a cleanup job.
- `log-sidecar/` — distroless `tail` image the operator can attach to AIS pods as a log sidecar.
- `docs/` — documentation covering deployment, auth, storage, networking, and troubleshooting (see `docs/README.md`).
- `local/` — `kind`-based local development cluster. `test-cluster.sh` is the entry point; `start-kind.sh`, `cluster-setup.sh`, and `delete-cluster.sh` cover the individual steps.
- `monitoring/` — Helm charts for the observability stack (kube-prometheus, Loki, Alloy, kube-state-metrics).
- `manifests/` — standalone manifests for cloud providers and Multus networking.
- `tools/` — node bootstrap scripts (`cloud-init`) and a Helm chart that runs maintenance jobs on nodes (`remote-exec`).

## Setup Commands

Most work happens inside `operator/`, a self-contained Go module. See [operator/AGENTS.md](operator/AGENTS.md) for its setup, build, and test commands. `make help` lists only the documented targets; read `operator/Makefile` for the E2E and kind targets.

Other components have their own local dependency setup; check the nearest `README.md` before editing them.

## Pull Request Guidelines

Full process: `CONTRIBUTING.md`. Key points:

- Commits must be signed off (`git commit -s`); unsigned commits cannot be merged.
- Squash multi-commit PRs into one commit before merge.
- If a change affects the operator's user-facing behavior, add an entry to `operator/CHANGELOG.md` under `Unreleased` in the same commit.
- Run `make lint`, `make fmt-check`, `make check-gen`, and `make test` in `operator/` before opening a PR. `make check-gen` compares against `HEAD`, so commit first.

## Security Considerations

- Read `SECURITY.md` and `docs/auth_profile.md#trust-model` before changing RBAC, auth flows, or CI workflow permissions — they define the trust boundaries this repo relies on.
- Report vulnerabilities privately per `SECURITY.md`; do not open a public issue or PR for a security finding.
