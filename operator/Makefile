# Current Operator version
VERSION ?= 1.1.0
BUILD_VERSION = $(VERSION)+$(shell git rev-parse --short HEAD)
BUILD_TIME = $(shell date +%FT%T%z)
REPO = https://github.com/NVIDIA/ais-k8s

# Default bundle image tag
BUNDLE_IMG ?= ais-operator-bundle:v$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

LOCALBIN=$(shell pwd)/bin
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

SHELL := /bin/bash
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
SCRIPTS_DIR = ./scripts

HELM_VERSION = v3.14.2

GOLANGCI_LINT_VERSION := v1.58.1
GOLANGCI_LINT := $(GOBIN)/golangci-lint
GOLANGCI_INSTALLER := https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh

all: manager


require-img:
	@[ "${IMG}" ] || ( echo "IMG must be set for this make target"; exit 1 )


# Run tests
test: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh"

test-short: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh" short

test-long: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh" long

test-manual: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh" manual

test-suite-deps:
	go env -w GOFLAGS=-mod=mod
	go install github.com/onsi/ginkgo/v2/ginkgo

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: require-img manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	@$(SHELL) -c "$(SCRIPTS_DIR)/deploy.sh --build"

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
undeploy:
	$(KUSTOMIZE) build config/default | kubectl delete -f -

deploy-metallb: kustomize
	@if [[ -z "$EXTERNAL_ADDRESSES" ]]; then echo "EXTERNAL_ADDRESSES variable cannot be empty"; exit 1; fi
	$(KUSTOMIZE) build github.com/metallb/metallb//manifests?ref=v0.9.5 | kubectl apply -f -
	@envsubst <config/metallb/configmap.template.yaml | kubectl apply -f -

undeploy-metallb: kustomize
	@kubectl delete cm metallb-config -n metallb-system
	$(KUSTOMIZE) build github.com/metallb/metallb//manifests?ref=v0.9.5 | kubectl delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

bundle-manifests: require-img kustomize manifests
	mkdir -p dist/
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/ais-operator.yaml

bundle-manifests-helm: helm helmify bundle-manifests
	mkdir -p dist/charts/
	cat dist/ais-operator.yaml | $(HELMIFY) dist/charts/ais-operator
	$(HELM) package dist/charts/ais-operator -d dist/
	$(HELM) repo index --url $(REPO)/releases/download/$(VERSION) dist/

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

## Update the linter version (removes previous one and downloads a new one)
lint-update-latest:
	rm -f $(GOLANGCI_LINT)
	curl -sSfL $(GOLANGCI_INSTALLER) | sh -s -- -b $(GOBIN) latest

## Install specific `golangci-lint` version
lint-update-fixed:
	@rm -f $(GOLANGCI_LINT)
	curl -sSfL $(GOLANGCI_INSTALLER) | sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION)

## Run linter on whole project
lint:
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" lint

lint-fix:
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" lint --fix

fmt-check: ## Check code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt

fmt-fix: ## Fix code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt --fix

spell-check: ## Run spell checker on the project
	@go install github.com/client9/misspell/cmd/misspell@v0.3.4
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" spell

spell-fix: ## Fix spell checking issues
	@go install github.com/client9/misspell/cmd/misspell@v0.3.4
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" spell --fix

# Build the docker image
docker-build: require-img generate fmt vet manifests
	docker build --build-arg BUILD_TIME="$(BUILD_TIME)" --build-arg BUILD_VERSION="$(BUILD_VERSION)" -t ${IMG} .

# Build an image accessible from a local k8s cluster (for testing)
docker-build-k8s: require-img docker-build
	docker save ${IMG} -o ais-operator-tmp.tar
	sudo ctr -n=k8s.io images import ais-operator-tmp.tar
	rm ais-operator-tmp.tar

# Push the docker image
docker-push: require-img
	docker push ${IMG}

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0)

# Download kustomize locally if necessary
KUSTOMIZE = $(LOCALBIN)/kustomize
.PHONY: kustomize
kustomize:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.7)

HELM = $(LOCALBIN)/helm
.PHONY: helm
helm:
	$(call helm-get-tool)

# Install helmify for generating helm chart from kustomize-generated manifests
HELMIFY = $(LOCALBIN)/helmify
.PHONY: helmify
helmify:
	$(call go-get-tool,$(HELMIFY),github.com/arttor/helmify/cmd/helmify@latest)


# go-get-tool will 'go get' any package $2 and install it to bin/ if $1 does not exist.
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(LOCALBIN) go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# get helm and install it locally
define helm-get-tool
@[ -f $(HELM) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
echo "Downloading Helm $(HELM_VERSION)" ;\
curl -sSL https://get.helm.sh/helm-$(HELM_VERSION)-linux-amd64.tar.gz -o helm.tar.gz ;\
tar -zxf helm.tar.gz ;\
mv linux-amd64/helm $(LOCALBIN) ;\
rm -rf $$TMP_DIR ;\
echo "Helm $(HELM_VERSION) has been installed to $(LOCALBIN)" ;\
}
endef

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: require-img manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .


require-init-img:
	@[ "${INIT_IMG}" ] || ( echo "INIT_IMG must be set for this make target"; exit 1 )

aisinit:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build  -ldflags "-w -s -X 'main.build=${BUILD_VERSION}' -X 'main.buildtime=${BUILD_TIME}'" -a -o bin/aisinit cmd/aisinit/main.go

docker-build-aisinit: require-init-img
	docker build -f cmd/aisinit/Dockerfile -t ${INIT_IMG} .

docker-push-aisinit:
	docker push ${INIT_IMG}
