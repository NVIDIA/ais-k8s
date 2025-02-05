CONTAINER_TOOL ?= docker
REGISTRY ?= docker.io
REPO_NS ?= aistorage
REPO_NAME ?= ais-logs
TAG ?= v1.0
IMG ?= $(REGISTRY)/$(REPO_NS)/$(REPO_NAME):$(TAG)

.PHONY: all
all: build push

.PHONY: build
build:
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: push
push:
	$(CONTAINER_TOOL) push ${IMG}
