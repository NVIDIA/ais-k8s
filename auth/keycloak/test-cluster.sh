#!/usr/bin/env bash
set -e

#### DO NOT USE FOR PRODUCTION ####

# This script
# 1. Creates a kind cluster
# 2. Installs all necessary prerequisites for keycloak
# 3. Installs keycloak
# 4. Imports AIS realm

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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
until kubectl get keycloak keycloak-server -n keycloak; do
  echo "Waiting for keycloak-server custom resource to exist..."
  sleep 5
done
echo "Waiting for keycloak to be ready (takes some time)..."
kubectl wait --for=condition=Ready --timeout=180s keycloak/keycloak-server -n keycloak

# TODO: Run this only when necessary
# Run import realm job
./realm/import-realm.sh

# Print initial temp admin credentials
USER=$(kubectl get secret -n keycloak keycloak-server-initial-admin -o jsonpath='{.data.username}' | base64 --decode)
PASS=$(kubectl get secret -n keycloak keycloak-server-initial-admin -o jsonpath='{.data.password}' | base64 --decode)

# Start a port forward and kill at the end of the script
kubectl port-forward -n keycloak service/keycloak-server-service 8543:8543 >/dev/null 2>&1 &
pid=$!
trap "kill $pid" EXIT

# Get ca.crt for trust from the issuer
CA_FILE=$SCRIPT_DIR/scripts/ca.crt
kubectl get secret ca-root-secret -n cert-manager -o "jsonpath={.data['ca\.crt']}" | base64 -d > "$CA_FILE"
# Create an ais-admin user
KEYCLOAK_HOST="https://keycloak-server-service.keycloak.svc.cluster.local:8543"
"$SCRIPT_DIR/scripts/prepare_cluster.sh" "$KEYCLOAK_HOST" "$USER" "$PASS" "$CA_FILE"

echo ""
echo "Initial admin user: ${USER}"
echo "Initial admin password: ${PASS}"
echo ""
echo "Port forward https through Traefik 'kubectl port-forward -n traefik service/traefik 8443:443'"
echo "Add the keycloak hostname to your hosts, e.g. 127.0.0.1  keycloak.local"
echo "curl -k https://keycloak.local:8443/realms/aistore"
