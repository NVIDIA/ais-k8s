#!/bin/bash

set -e

# TODO: It seems like a good idea to remember stuff that user entered so that
#  using `destroy.sh` would be automatic. For example we could remember cloud,
#  project ids, flags as well as created `kubectl` contexts.

source utils.sh

check_command terraform
check_command kubectl

check_providers

if [[ ${cloud_provider} == "aws" ]]; then
  print_error "'aws' provider not yet supported"
  check_command aws

  # TODO: Check if `aws` is initialized with project id and region.
  aws configure
  terraform_args=()
elif [[ ${cloud_provider} == "azure" ]]; then
  print_error "'azure' provider not yet supported"
  terraform_args=()
elif [[ ${cloud_provider} == "gcp" ]]; then
  check_command gcloud

  # Check if user is logged into `gcloud`.
  if [[ -z $(gcloud config list account --format "value(core.account)") ]]; then
    gcloud init --console-only
  fi

  # Check if project ID is set. If it is then use it as input for the terraform.
  project_id=$(gcloud config get-value core/project)
  if [[ -z ${project_id} ]]; then
    print_error "project id is not set in 'gcloud'"
  fi
  terraform_args=(-var "project_id=${project_id}")
fi

# Initialize terraform and download necessary plugins.
terraform init -input=false "${cloud_provider}"

# Execute terraform plan. The approved automatically as we assume that everything is correct.
echo "Starting Kubernetes cluster..."
terraform apply -input=false -auto-approve "${terraform_args[@]}" "${cloud_provider}"

echo "Updating kubectl config..."
if [[ ${cloud_provider} == "aws" ]]; then
  aws eks --region us-east-2 update-kubeconfig --name training-eks-sR8eLIil
elif [[ ${cloud_provider} == "azure" ]]; then
  :
elif [[ ${cloud_provider} == "gcp" ]]; then
  gcloud container clusters get-credentials "$(terraform output kubernetes_cluster_name)" --region "$(terraform output region)"
  echo "kubectl configured to use '$(kubectl config current-context)' context"
fi


echo "Setting up kubectl..."
kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/v2.0.0-beta8/aio/deploy/recommended.yaml
kubectl proxy
kubectl -n kube-system describe secret "$(kubectl -n kube-system get secret | grep service-controller-token | awk '{print $1}')"

echo "Deploying AIStore on the cluster"
# kubectl apply -f
