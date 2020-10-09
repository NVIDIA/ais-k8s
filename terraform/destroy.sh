#!/bin/bash

set -eo pipefail

source utils.sh

stop_k8s() {
  echo -e "☠️  Destroying..."

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

  terraform destroy -auto-approve "${terraform_args[@]}" "${cloud_provider}"

  echo -e "\n☠️  Stopping 'kubectl proxy'..."
  killall kubectl proxy

  echo -e "\n❌ Unsetting kubectl context..."
  context="$(kubectl config get-contexts | grep 'ais' | awk '{print $2}')"
  if [[ -n ${context} ]]; then
    kubectl config unset "contexts.${context}"
  fi
}

stop_ais() {
  echo "☠️  Stopping AIStore cluster..."
  helm uninstall demo
  remove_nodes_labels
}


case $1 in
--all)
  check_command terraform
  check_command kubectl
  check_command killall
  check_command helm

  check_providers

  stop_ais
  stop_k8s
  ;;
--ais)
  check_command helm

  stop_ais
  ;;
*)
  print_error "unknown argument provided"
  ;;
esac

