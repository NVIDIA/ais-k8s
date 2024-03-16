# Create an image including tests for running on a pod inside a local k8s cluster (e.g. minikube)
FROM aistorage/ci:latest
WORKDIR /operator
COPY ais-k8s/operator .

# Install make
RUN apt update && apt install -y make
CMD make test-short