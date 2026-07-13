CONTAINER_TOOL ?= docker
REGISTRY ?= docker.io
REPO_NS ?= aistorage
REPO_NAME ?= ais-logs
TAG ?= v1.2
IMG ?= $(REGISTRY)/$(REPO_NS)/$(REPO_NAME):$(TAG)
TARGET_PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: all
all: buildx-push

.PHONY: build
build:
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: push
push:
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: buildx-push
buildx-push:
	$(CONTAINER_TOOL) buildx build --platform $(TARGET_PLATFORMS) --tag ${IMG} --push .
