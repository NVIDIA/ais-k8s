#!/bin/bash

set -e

source utils.sh

check_command terraform
check_command kubectl

check_providers

echo "Destroying..."

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

  # Check if project ID is set. If it is then use it as input for the terraform.
  project_id=$(gcloud config get-value core/project)
  if [[ -z ${project_id} ]]; then
    print_error "project id is not set in 'gcloud'"
  fi
  terraform_args=(-var "project_id=${project_id}")
fi

terraform destroy -auto-approve "${terraform_args[@]}" -var-file="${cloud_provider}/terraform.tfvars" "${cloud_provider}"

echo -e "\nUnsetting kubectl context..."
context="$(kubectl config get-contexts | grep 'ais' | awk '{print $2}')"
if [[ -n ${context} ]]; then
  kubectl config unset "contexts.${context}"
fi
