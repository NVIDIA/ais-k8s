#!/bin/bash

set -e

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

    project_id=$(get_state_var "GKE_PROJECT_ID")
    username=$(get_state_var "GKE_USERNAME")
    node_cnt=$(get_state_var "NODE_CNT")

    terraform_args=(-var "project_id=${project_id}" -var "user=${username}"  -var "node_count=${node_cnt}")
  fi

  if [[ -n $(get_state_var "VOLUMES_DEPLOYED") ]]; then
    pushd k8s/ 1>/dev/null
    terraform destroy -auto-approve "${cloud_provider}"
    popd 1>/dev/null
    unset_state_var "VOLUMES_DEPLOYED"
  fi
  terraform destroy -auto-approve "${terraform_args[@]}" "${cloud_provider}"

  echo -e "\n☠️  Stopping 'kubectl proxy'..."
  killall kubectl proxy || true

  echo -e "\n❌ Unsetting kubectl context..."
  context="$(kubectl config get-contexts | grep 'ais' | awk '{print $2}')"
  if [[ -n ${context} ]]; then
    kubectl config unset "contexts.${context}"
  fi
}

stop_ais() {
  if [[ -z $(get_state_var "AIS_DEPLOYED") ]]; then
    return
  fi

  echo "☠️  Stopping AIStore cluster..."
  helm uninstall demo
  kubectl delete pvc --all # TODO: We should reuse them on restart.
  kubectl delete pv --all
  remove_nodes_labels
  unset_state_var "AIS_DEPLOYED"
}


case $1 in
--all)
  check_command terraform
  check_command kubectl
  check_command helm
  check_command killall

  cloud_provider=$(get_state_var "CLOUD_PROVIDER")
  if [[ -z ${cloud_provider} ]]; then
    print_error "cloud provider is not set, make sure that you've deployed the cluster"
  fi

  stop_ais
  stop_k8s
  remove_state_file
  ;;
--ais)
  check_command kubectl
  check_command helm

  stop_ais
  ;;
*)
  print_error "unknown argument provided"
  ;;
esac

