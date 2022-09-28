# Current Operator version
VERSION ?= 0.9.3
BUILD_VERSION = $(VERSION).$(shell git rev-parse --short HEAD)
BUILD_TIME = $(shell date +%FT%T%z)

# Default bundle image tag
BUNDLE_IMG ?= ais-operator-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= aistore/ais-operator:latest
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

all: manager

# Run tests
test: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh"

test-short: generate fmt vet manifests test-suite-deps
	@$(SHELL) "$(SCRIPTS_DIR)/test.sh" short

test-suite-deps:
	go env -w GOFLAGS=-mod=mod
	go install github.com/onsi/ginkgo/ginkgo

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
deploy: manifests kustomize
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
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

bundle-manifests: manifests
	mkdir -p dist/
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/ais-operator-${VERSION}.yaml

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."


lint-update: ## Update the linter version (removes previous one and downloads a new one)
	@rm -f $(GOPATH)/bin/golangci-lint
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin latest

## Install specific `golangci-lint` version (hardcoded below)
## See also: .github/workflows/lint.yml
lint-update-ci:
	@rm -f $(GOPATH)/bin/golangci-lint
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin v1.49.0

lint: ## Run linter on whole project
	@([[ ! -f $(GOPATH)/bin/golangci-lint ]] && curl -sfL "https://install.goreleaser.com/github.com/golangci/golangci-lint.sh" | sh -s -- -b $(GOPATH)/bin latest) || true
	golangci-lint run
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" lint

fmt-check: ## Check code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt

fmt-fix: ## Fix code formatting
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" fmt --fix

spell-check: ## Run spell checker on the project
	@GOOS="" GO111MODULE=off go get -u github.com/client9/misspell/cmd/misspell
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" spell

spell-fix: ## Fix spell checking issues
	@GOOS="" GO111MODULE=off go get -u github.com/client9/misspell/cmd/misspell
	@$(SHELL) "$(SCRIPTS_DIR)/lint.sh" spell --fix

# Build the docker image
docker-build: generate fmt vet manifests
	docker build --build-arg BUILD_TIME="$(BUILD_TIME)" --build-arg BUILD_VERSION="$(BUILD_VERSION)" -t ${IMG} .

# Push the docker image
docker-push:
	docker push ${IMG}

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# Download kustomize locally if necessary
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.2)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
