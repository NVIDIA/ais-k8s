# AGENTS.md (operator)

See the [root AGENTS.md](../AGENTS.md) for repo-wide setup and PR guidelines. This file adds rules specific to `operator/`.

## Layout

- `api/aistore/v1beta1/` — `AIStore` CRD types and validation.
- `api/aisauth/v1alpha1/` — `AIStoreAuth` and `AIStoreAuthProfile` CRD types and validation.
- `internal/controller/` — reconcile loops, one subdirectory per CRD group (`aistore`, `aisauth`).
- `internal/resources/` — builds the K8s objects the controllers apply: `aistore/{proxy,target,adminclient,cmn}`, `aisauth/` (AuthN deployment, ConfigMap, services, volumes, plus its `config/` types), `certificates`, `ownerref`.
- `internal/services/` — clients the operator uses at runtime: AIS API (`aisapi.go`), auth token exchange (`auth_api.go`, `auth_profile.go`), TLS client config.
- `internal/webhook/` — admission webhooks, one subdirectory per CRD group (`aistore/v1beta1`, `aisauth/v1alpha1`).
- `internal/client/`, `internal/svcaddr/`, `internal/truststore/`, `internal/opinfo/` — smaller shared helpers (K8s client wrapper, service address resolution, CA trust pool, cluster domain and operator identity).
- `config/` — kustomize manifests. `make manifests` writes generated CRD, RBAC, and webhook YAML into `config/base/`. `config/overlays/` holds the `default`, `helm`, `certmanager`, `metrics`, and `test` overlays. `config/samples/` holds example CRs, one per resource or scenario.
- `cmd/main.go` — entrypoint.
- `tests/` — E2E specs (`e2e/`), shared test helpers (`tutils/`), in-cluster CI runner (`ci/`).
- `scripts/` — shell helpers the Makefile calls for tests, lint, and tool installation.

## Development Workflow (Operator)

```bash
cd operator
go mod download
```

Make targets:

```bash
make build      # go build the manager binary
make run        # run the controller against the cluster in ~/.kube/config
make manifests  # regenerate CRD/RBAC/webhook YAML from kubebuilder markers
make generate   # regenerate deepcopy/mock code
```

`build`, `run`, `test`, and `test-e2e` each depend on `manifests` and `generate`, so they rewrite generated files. `build` and `run` also depend on `fmt` and `vet`, so they reformat the tree.

If you change types under `api/`, run `make generate` and `make manifests`, then regenerate the Helm chart. Pass the version the chart already declares:

```bash
make build-installer-helm VERSION=<current version in helm/ais-operator/Chart.yaml>
```

`build-installer-helm` rewrites `.version` and `.appVersion` in `helm/ais-operator/Chart.yaml` and the image tag in `config/overlays/default/kustomization.yaml`. Pass a different version only during a release. `make build-installer` writes `dist/ais-operator.yaml`, which is gitignored and needed only at release time.

## Testing Instructions

Run from `operator/`. Full details: `tests/README.md`.

Unit tests live beside the code they cover, as one ginkgo suite per package (`*_suite_test.go` bootstraps the suite). A `*_internal_test.go` file uses the package under test for white-box coverage.

```bash
make test                                       # unit tests
make kind-setup                                 # create a local kind cluster for E2E
make test-e2e-bootstrap                         # install E2E test dependencies
make test-e2e-in-cluster                        # run E2E tests inside the cluster
AIS_TEST_API_MODE=public make test-e2e          # run E2E tests from outside the cluster
TEST_E2E_MODE=manual make test-e2e-in-cluster   # include ginkgo `manual`-labeled specs
make test-e2e-teardown                          # tear down E2E dependencies
make kind-teardown                              # delete the kind cluster
```

E2E test behavior is configurable via environment variables (test images, storage class, API mode) — see the table in `tests/README.md`.

To iterate on a single package or spec, call ginkgo directly instead of `make test`:

```bash
ginkgo ./internal/services                          # one package
ginkgo --focus "<spec text>" ./internal/services    # one spec
```

The suites in `internal/controller/aistore/` and `api/aistore/v1beta1/` start envtest and need `KUBEBUILDER_ASSETS`; use `make test` for those.

## Code Style Guidelines

Run from `operator/`:

```bash
make lint       # golangci-lint
make lint-fix   # golangci-lint with autofix
make fmt-check  # go mod tidy check, copyright headers, import block spacing
make fmt-fix    # gofmt -s -w only
make check-gen  # verify config/ and helm/ais-operator/ match the generators
```

`make fmt-fix` fixes nothing that `make fmt-check` reports. Fix those by hand: run `go mod tidy`, add the file header, or collapse the blank lines in the `import` block.

- Every `.go` file needs a `Copyright ... NVIDIA CORPORATION. All rights reserved.` line within its first 10 lines, or `// no-copyright`. Copy the header from a neighboring file.
- An `import` block may hold at most one blank line.

`make check-gen` diffs `config/` and `helm/ais-operator/` against `HEAD`, so commit regenerated manifests before you run it.

CI runs `make lint`, `make fmt-check`, and `make check-gen` on every operator change — run these locally before committing.

## Build and Deployment

- CI pushes the `aistorage/ais-operator` image on a `v*` tag push and via manual workflow dispatch.
- `../pages/` holds the published Helm chart repo.
- Release process: `../docs/operator_release.md`. `VERSION=<x.y.z> make release` also creates the release commit, so do not run it unless you intend that.
- Changing the supported AIStore or Kubernetes versions also requires an update to `../docs/COMPATIBILITY.md`.

## Comments

Write a comment only to state intent or a constraint the code cannot show. Never narrate what the code does.

Doc comments (package, type, function, method) describe only that entity's own responsibility:

- Never describe internals, precedence/fallback order, or what the code inside them does — that detail changes and the comment goes stale.
- Never make claims about callers, deployment, external tooling, or where a value is consumed later.
- Never justify a design or a change, and never reference obsolete or unimplemented behavior.

Comment specific confusing code inline, immediately above the line it explains, not in the doc comment. Keep each comment to the shortest form that still carries the constraint, and match the surrounding file's comment density and style.

```go
// BAD: describes internals, fallbacks, and callers
// Resolve determines the domain and identity, logging each value and its source. Neither failure is
// fatal: the domain falls back to the default, and callers that never need the identity keep running.

// GOOD: states its own responsibility and contract
// Resolve determines the cluster domain and the operator's own identity. It reports failures through
// the logger rather than to the caller.
```

Write comments in [ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/):

- Use the active voice. Use simple present, simple past, or simple future tense.
- Write one instruction per sentence. Keep instruction sentences to 20 words or fewer and descriptive sentences to 25 words or fewer.
- Use each approved word in one part of speech and one meaning only. Use the same word for the same thing every time.
- Do not use noun clusters of more than three words.
- Do not omit articles or other words that make the sentence clear.
