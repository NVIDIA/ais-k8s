# Create an image including tests for running on a pod inside a local k8s cluster (e.g. minikube)
FROM aistorage/ci:latest
WORKDIR /operator
COPY / .

# Use an entrypoint script to run tests
ENTRYPOINT ["/operator/tests/entrypoint.sh"]
