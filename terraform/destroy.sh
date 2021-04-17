#!/bin/bash

set -e

source scripts/utils.sh
source scripts/volumes.sh

stop_k8s() {
  echo -e "☠️  Destroying..."

  if [[ ${cloud_provider} == "aws" ]]; then
    print_error "'aws' provider is not yet supported"

    check_command aws

    # TODO: Check if `aws` is initialized with project id and region.
    aws configure
    terraform_args=(-var "node_count=${node_cnt}" -var "ais_release_name=${release_name}")
  elif [[ ${cloud_provider} == "azure" ]]; then
    print_error "'azure' provider is not yet supported"

    terraform_args=(-var "node_count=${node_cnt}" -var "ais_release_name=${release_name}")
  elif [[ ${cloud_provider} == "gcp" ]]; then
    if [[ $preserve_disks = true ]]; then
      echo "💾 Saving locally persistent storage metadata for future use"
      locally_persist_volumes

      # Remove leftovers, they will be recreated based on just created yamls.
      kubectl delete pvc --all
      kubectl delete pv --all
    fi

    check_command gcloud

    project_id=$(get_state_var "GKE_PROJECT_ID")
    username=$(get_state_var "GKE_USERNAME")
    node_cnt=$(get_state_var "NODE_CNT")

    terraform_args=(-var "project_id=${project_id}" -var "user=${username}" -var "node_count=${node_cnt}" -var "ais_release_name=${release_name}")
  fi

  terraform -chdir="${cloud_provider}" destroy -auto-approve "${terraform_args[@]}"

  echo -e "\n☠️  Stopping 'kubectl proxy'..."
  killall kubectl proxy || true

  echo -e "\n❌ Unsetting kubectl context..."
  context="$(kubectl config get-contexts | grep 'ais' | awk '{print $2}')"
  if [[ -n ${context} ]]; then
    kubectl config unset "contexts.${context}"
  fi

  if [[ $preserve_disks = true ]]; then
    echo "❗ Kubernetes cluster was destroyed, but persistent volumes were not. If you no longer want to use them, remove $pv_dir and $pvc_dir directories."
  fi
}

get_disks() {
  nodes=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')
  disks=$(gcloud compute instances list --zones="$(terraform_output zone)" --format="value(disks[].deviceName)" --filter="name:(${nodes})" | tr ";" "\n" | grep "gke-ais" || true)
  echo $disks
}

stop_ais() {
  if [[ -z $(get_state_var "AIS_DEPLOYED") ]]; then
    return
  fi

  echo "☠️  Stopping AIStore cluster..."
  cloud_provider=$(get_state_var "CLOUD_PROVIDER")

  if [[ ${cloud_provider} == "gcp" ]]; then
    disks=$(get_disks)
  fi

  # Manually remove ETL Pods and Services (if exist).
  # TODO: Eventually we should make this automatic (probably handled by `helm`).
  kubectl delete pods,services -l nvidia.com/ais-etl-name

  helm uninstall demo || true

  if [[ $preserve_disks = false ]]; then
    kubectl delete pvc --all
    kubectl delete pv --all

    # Make sure that there are no leftovers.
    clear_persisted_volumes
  fi

  if [[ -n $(get_state_var "VOLUMES_DEPLOYED") ]]; then
    pushd k8s/ 1>/dev/null
    terraform -chdir="${cloud_provider}" destroy -auto-approve
    popd 1>/dev/null
    unset_state_var "VOLUMES_DEPLOYED"
  fi

  # Do not remove PV and PVC is preserve_disks, they will be used with the next deployment.
  if [[ $preserve_disks = false ]] && [[ ${cloud_provider} == "gcp" ]]; then
    if [[ ${#disks} -ne 0 ]]; then
      for ((i=0; i < 10; i++)); do
        current_disks=$(get_disks)
        if [[ ${#current_disks} -ne 0 ]]; then
          printf "Waiting for disks to be unattached from cluster nodes\n"
          sleep 5
          continue
        fi
        break
      done

      # If zone don't match with the cluster's zone then a disk won't be deleted.
      printf "y\n" | gcloud compute disks delete $disks --zone "$(terraform_output zone)" 1>/dev/null || true
    fi
  fi

  remove_nodes_labels
  unset_state_var "AIS_DEPLOYED"
}

print_help() {
  printf "%-15s\tStarts nodes on specified provider, starts K8s cluster and deploys AIStore on K8s nodes.\n" "all"
  printf "%-15s\tOnly deploy AIStore on K8s nodes, assumes that K8s cluster is already deployed.\n" "ais"
  printf "%-15s\tShows this help message.\n" "--help"
}

destroy_type=$1; shift
preserve_disks=false

while (( "$#" )); do
  case "$1" in
    --preserve-disks) preserve_disks=true; shift;;
    --help) print_help; exit 0;;
    *) echo "fatal: unknown argument '$1'"; exit 1;;
  esac
done

if [[ $preserve_disks = true ]]; then
  echo "⚠️ Created persistent disks will not be removed. This may create additional storage costs."
fi

case ${destroy_type} in
all)
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
ais)
  check_command kubectl
  check_command helm

  stop_ais
  ;;
--help)
  print_help
  ;;
*)
  print_error "invalid destroy type: '${destroy_type}' (expected 'all' or 'ais')"
  ;;
esac

