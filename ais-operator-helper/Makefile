TAG ?= latest
REGISTRY_URL ?= docker.io/aistorage

all: build push

build:
	docker build -t $(REGISTRY_URL)/ais-operator-helper:$(TAG) .

push:
	docker push $(REGISTRY_URL)/ais-operator-helper:$(TAG)