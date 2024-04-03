export MINIKUBE_HOME=${MINIKUBE_HOME:-/var/local/minikube/.minikube}
MINIKUBE_PORT=8443
MINIKUBE_HOST=https://$(minikube ip)

# Build using minikube docker daemon
eval $(minikube docker-env)

# Build a docker image based on the current local repo
# If you change this name, be sure to update it in test_job_spec.yaml.template as well
IMG=operator-test:latest
# build from the root of the directory to include the context
docker build -t $IMG -f Dockerfile ../../../ --no-cache

# Created by test_job_spec.yaml
JOB=operator-test

# Apply the pod spec to deploy the image in a pod in the local k8s cluster
NAMESPACE=default
KUBE_SERVER=$MINIKUBE_HOST:$MINIKUBE_PORT
KUBE_CERT=$MINIKUBE_HOME/profiles/minikube/apiserver.crt
KUBE_KEY=$MINIKUBE_HOME/profiles/minikube/apiserver.key
KUBE_CA=$MINIKUBE_HOME/ca.crt
kubectl config set-cluster minikube-cluster --server=$KUBE_SERVER --certificate-authority=$KUBE_CA
kubectl config set-credentials minikube-user --client-certificate=$KUBE_CERT --client-key=$KUBE_KEY
kubectl config set-context minikube-context --cluster=minikube-cluster --user=minikube-user --namespace=default
kubectl config use-context minikube-context
# Delete any previously running test jobs
kubectl delete job $JOB --ignore-not-found=true

# Select the type of test to run
TEST_TYPE=${1:-""}
export TEST_TYPE
envsubst < test_job_spec.yaml.template > test_job_spec.yaml

# Start the test job
kubectl apply -f test_job_spec.yaml
# Get all pods for the job and their deletion timestamps
PODS_JSON=$(kubectl get pods --selector=job-name=$JOB -o json)
# Use jq to parse the JSON and filter out pods that are terminating
POD_NAME=$(echo $PODS_JSON | jq -r '.items[] | select(.metadata.deletionTimestamp == null) | .metadata.name')
echo "Started test"
echo -e "To view logs: \tkubectl logs $POD_NAME --follow"
echo -e "To abort: \tkubectl delete job $JOB"