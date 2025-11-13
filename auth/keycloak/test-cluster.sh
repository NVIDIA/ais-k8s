#!/usr/bin/env bash
set -e

# This script
# 1. Creates a kind cluster
# 2. Installs all necessary prerequisites for keycloak
# 3. Installs keycloak
# 4. Imports AIS realm

CLUSTER_NAME="keycloak-test"

if kind get clusters | grep -qw "${CLUSTER_NAME}"; then
  echo "Cluster ${CLUSTER_NAME} already exists, skipping creation."
else
  kind create cluster --config=./kind/config.yaml --name=${CLUSTER_NAME}
fi

# Verify we are running with the right context
CURRENT=$(kubectl config current-context)
if [ "${CURRENT}" != "kind-${CLUSTER_NAME}" ]; then
  echo "Warning: kubectl context does not match new KinD cluster!"
  exit 1
fi

# Install pre-reqs -- storage class, ingress etc.
helmfile -f prereq-helmfile.yaml sync
# Create namespace but allow existing
kubectl create namespace keycloak || true

# Install cluster issuer for making cert
helmfile -f ../../helm/cluster-issuer/helmfile.yaml sync
# Create a certificate
kubectl apply -f manifests/certificate.yaml

# Install cnpg
helmfile -f ./cnpg/helm/operator/helmfile.yaml sync
echo "Waiting for cnpg controller webhook to become available..."
kubectl rollout status deployment/cloudnative-pg-operator -n cnpg-system --timeout=120s
helmfile -f ./cnpg/helm/cluster/helmfile.yaml sync

#### KEYCLOAK ####
# CRDs
kubectl apply -f https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/26.4.2/kubernetes/keycloaks.k8s.keycloak.org-v1.yml
kubectl apply -f https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/26.4.2/kubernetes/keycloakrealmimports.k8s.keycloak.org-v1.yml

# Operator
kubectl -n keycloak apply -f https://raw.githubusercontent.com/keycloak/keycloak-k8s-resources/26.4.2/kubernetes/kubernetes.yml

# Create a secret for keycloak to access the DB, allow existing
kubectl create secret -n keycloak generic keycloak-db-secret --from-literal=username=app --from-literal=password="$(kubectl get secret cloudnative-pg-cluster-app --namespace cnpg-database -o jsonpath='{.data.password}' | base64 --decode)" || true

# Manifest
kubectl apply -f manifests/keycloak.yaml
echo "Waiting for keycloak to be ready (takes some time)..."
kubectl wait --for=condition=Ready --timeout=180s keycloak/keycloak-server -n keycloak

# Run import realm job
./realm/import-realm.sh

# Print initial temp admin credentials
USER=$(kubectl get secret -n keycloak keycloak-server-initial-admin -o jsonpath='{.data.username}' | base64 --decode)
PASS=$(kubectl get secret -n keycloak keycloak-server-initial-admin -o jsonpath='{.data.password}' | base64 --decode)

echo "Initial user: ${USER}"
echo "Initial password: ${PASS}"
