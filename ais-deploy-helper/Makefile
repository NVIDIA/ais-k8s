IMAGE_NAME=aistorage/ais-deploy-helper
IMAGE_TAG=0.1.0
IMG ?= ${IMAGE_NAME}:${IMAGE_TAG}

# Build the docker image
build:
	docker build -t ${IMG} .

# Push the docker image
push:
	docker push ${IMG}
