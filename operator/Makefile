DEFAULT_VERSION = 2.1.2
# Strip off any preceding `v` from version string
HELM_CHART_VERSION := $(shell echo $(VERSION) | sed 's/^v//')
# Use VERSION if provided
BUILD_VERSION_BASE = $(if $(VERSION),$(VERSION),$(DEFAULT_VERSION))
BUILD_VERSION = $(BUILD_VERSION_BASE)+$(shell git rev-parse --short HEAD)
BUILD_TIME = $(shell date +%FT%T%z)

REPO = https://github.com/NVIDIA/ais-k8s
PAGES = https://NVIDIA.github.io/ais-k8s/charts

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.25.0

# KIND_K8S_VERSION refers to the version of container images for nodes that are being created during cluster setup.
KIND_K8S_VERSION ?= v1.28.7

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

SCRIPTS_DIR = ./scripts

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

require-img:
	@[ "${IMG}" ] || ( echo "IMG must be set for this make target"; exit 1 )

require-version:
	@[ "${VERSION}" ] || ( echo "VERSION must be set for this make target"; exit 1 )

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen mockgen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	go generate ./...

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test test-e2e test-e2-short test-e2-long test-e2e-manual
test: manifests generate envtest ## Run tests.
	@[[ $$(command -v ginkgo) ]] || go install github.com/onsi/ginkgo/v2/ginkgo
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" ginkgo --vv --skip-package e2e --coverprofile cover.out ./...

test-e2e: manifests generate envtest
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(SHELL) "$(SCRIPTS_DIR)/test.sh"

test-e2e-short: manifests generate envtest
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(SHELL) "$(SCRIPTS_DIR)/test.sh" short

test-e2e-long: manifests generate envtest
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(SHELL) "$(SCRIPTS_DIR)/test.sh" long

test-e2e-manual: manifests generate envtest
	@KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(SHELL) "$(SCRIPTS_DIR)/test.sh" manual

.PHONY: kind-setup kind-teardown
kind-setup: kind
	$(KIND) create cluster --name ais-operator-test --image kindest/node:$(KIND_K8S_VERSION) --config scripts/kind_cluster_local.yaml

kind-teardown: kind
	$(KIND) delete cluster --name ais-operator-test

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: fmt-check
fmt-check: ## Check code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt

.PHONY: fmt-fix
fmt-fix: ## Fix code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: require-img ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: require-img ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name output-dir-builder
	$(CONTAINER_TOOL) buildx use output-dir-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm output-dir-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: require-img manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/ais-operator.yaml

.PHONY: build-installer-helm
build-installer-helm: require-img require-version helm helmify build-installer
	mkdir -p dist/ais-operator
	$(HELMIFY) -f dist/ais-operator.yaml dist/ais-operator
	$(HELM) package dist/ais-operator -d ../pages/charts/ --version $(HELM_CHART_VERSION) --app-version $(HELM_CHART_VERSION)
	$(HELM) repo index --url $(PAGES) ../pages/charts --merge ../pages/charts/index.yaml

.PHONY: release
release: build-installer-helm
	git add ../pages/charts config/manager/kustomization.yaml
	git commit -s -m "operator: Release version $(VERSION)"
	git tag $(VERSION)

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: require-img manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
HELMIFY = $(LOCALBIN)/helmify
# Install mockgen to GOPATH since it's called from within source files
MOCKGEN = $(GOPATH)/bin/mockgen
HELM = $(LOCALBIN)/helm
KIND ?= $(LOCALBIN)/kind
CLOUD_PROVIDER_KIND ?= $(LOCALBIN)/cloud-provider-kind

## Tool Versions
HELM_VERSION ?= v3.16.2
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.17.3
ENVTEST_VERSION ?= release-0.18
MOCKGEN_VERSION ?= v0.4.0
KIND_VERSION ?= v0.27.0
CLOUD_PROVIDER_KIND_VERSION ?= v0.6.0

# Lint
GOLANGCI_LINT = $(shell which golangci-lint)
GOLANGCI_LINT_INSTALLER = https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh
GOLANGCI_LINT_VERSION ?= v2.0.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	./scripts/go_install_tool.sh $(KUSTOMIZE) sigs.k8s.io/kustomize/kustomize/v5 $(KUSTOMIZE_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	./scripts/go_install_tool.sh $(CONTROLLER_GEN) sigs.k8s.io/controller-tools/cmd/controller-gen $(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	./scripts/go_install_tool.sh $(ENVTEST) sigs.k8s.io/controller-runtime/tools/setup-envtest $(ENVTEST_VERSION)

.PHONY: golangci-lint
golangci-lint:
# If we don't have it in path, install locally if necessary
ifeq ($(GOLANGCI_LINT),)
	curl -sSfL $(GOLANGCI_LINT_INSTALLER) | sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	$(eval GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint)
endif
	@echo "Using golangci-lint: $(GOLANGCI_LINT)"

.PHONY: helm
helm:
	./scripts/install_helm.sh $(LOCALBIN) $(HELM_VERSION)

.PHONY: helmify
helmify: $(HELMIFY)
$(HELMIFY): $(LOCALBIN)
	./scripts/go_install_tool.sh $(HELMIFY) github.com/arttor/helmify/cmd/helmify latest

.PHONY: mockgen
mockgen: $(MOCKGEN) ## Download mockgen locally if necessary.
$(MOCKGEN):
	./scripts/go_install_tool.sh $(MOCKGEN) go.uber.org/mock/mockgen $(MOCKGEN_VERSION)

.PHONY: kind
kind: $(KIND)
$(KIND): $(LOCALBIN)
	./scripts/go_install_tool.sh $(KIND) sigs.k8s.io/kind $(KIND_VERSION)

.PHONY: cloud-provider-kind
cloud-provider-kind: $(CLOUD_PROVIDER_KIND)
$(CLOUD_PROVIDER_KIND): $(LOCALBIN)
	./scripts/go_install_tool.sh $(CLOUD_PROVIDER_KIND) sigs.k8s.io/cloud-provider-kind $(CLOUD_PROVIDER_KIND_VERSION)
