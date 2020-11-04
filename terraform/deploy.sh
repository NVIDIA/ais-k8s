#!/bin/bash

set -eo pipefail

source utils.sh

deploy_ais() {
  echo "🔥 Deploying AIStore on the cluster"

  # Remove labels (from all nodes) if exist.
  remove_nodes_labels

  # Label nodes so they match a required selector.
  kubectl label nodes --all \
    nvidia.com/ais-target="${release_name}-ais" \
    nvidia.com/ais-proxy="${release_name}-ais-electable" \
    1>/dev/null

  # Label one of the nodes to mark it as a primary.
  primary_node=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
  kubectl label nodes "${primary_node}" \
    nvidia.com/ais-admin="${release_name}-ais" \
    nvidia.com/ais-initial-primary-proxy="${release_name}-ais" \
    1>/dev/null

  external_ip=$(terraform output external_ip)
  pushd ../helm/ais 1>/dev/null

  AIS_NAME="${release_name}" \
    AIS_GATEWAY_EXTERNAL_IP="${external_ip}" \
    AIS_K8S_CLUSTER_CIDR="10.64.0.0/14" \
    AISNODE_IMAGE="aistore/aisnode:3.3-k8s" \
    KUBECTL_IMAGE="gmaltby/ais-kubectl:1" \
    EXTERNAL_VOLUMES_COUNT="$(get_state_var "DISK_CNT")" \
    STATS_NODENAME="${primary_node}" \
    HELM_ARGS="--set tags.builtin_monitoring=false,tags.prometheus=false,aiscluster.expected_target_nodes=$(kubectl get nodes --no-headers | wc -l | xargs),aiscluster.skipHostIP=true,admin.enabled=true" \
    ./run_ais_sample.sh

  popd 1>/dev/null

  set_state_var "AIS_DEPLOYED" "true"
}

deploy_k8s() {
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

    username=$(gcloud config get-value account)
    if [[ -z ${username} ]]; then
      print_error "username is not set in 'gcloud'"
    fi

    set_state_var "GKE_PROJECT_ID" "${project_id}"
    set_state_var "GKE_USERNAME" "${username}"

    terraform_args=(-var "project_id=${project_id}" -var "user=${username}" -var "node_count=${node_cnt}" -var "ais_release_name=${release_name}")
  fi

  # Initialize terraform and download necessary plugins.
  echo "Initializing terraform cluster environment"
  terraform init -input=false "${cloud_provider}" 1>/dev/null

  # Execute terraform plan. The approved automatically as we assume that everything is correct.
  echo "🔥 Starting Kubernetes cluster (${username}/${project_id})..."
  terraform apply -input=false -auto-approve "${terraform_args[@]}" "${cloud_provider}"

  echo "🔄 Updating kubectl config..."
  if [[ ${cloud_provider} == "aws" ]]; then
    aws eks --region us-east-2 update-kubeconfig --name training-eks-sR8eLIil
  elif [[ ${cloud_provider} == "azure" ]]; then
    :
  elif [[ ${cloud_provider} == "gcp" ]]; then
    gcloud container clusters get-credentials "$(terraform output kubernetes_cluster_name)" --zone "$(terraform output zone)"
    echo "✅ kubectl configured to use '$(kubectl config current-context)' context"
  fi

  pushd k8s/ 1>/dev/null
  echo "Initializing persistent storage"
  terraform init -input=false "${cloud_provider}" 1>/dev/null
  terraform apply -input=false -auto-approve "${cloud_provider}"
  popd 1>/dev/null

  set_state_var "VOLUMES_DEPLOYED" "true"
}

deploy_dashboard() {
  echo "🔄 Setting up k8s dashboard..."
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/dashboard/v2.0.0-beta8/aio/deploy/recommended.yaml
  kubectl proxy &
  echo "✅ kubectl proxy started"
  echo "🌐 Visit: http://127.0.0.1:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/"
  echo "🔑 Use this token to authenticate: $(kubectl -n kube-system get secret/"$(kubectl -n kube-system get secret | grep service-controller-token | awk '{print $1}')" --template="{{.data.token}}" | base64 -D)"
}

print_help() {
  printf "%-15s\tStops K8s pods, and destroys started nodes.\n" "--all"
  printf "%-15s\tOnly stops AIStore Pods so the cluster can be redeployed.\n" "--ais"
  printf "%-15s\t\t\tShows this help message.\n" "--help"
}


case $1 in
--all)
  check_command terraform
  check_command kubectl
  check_command helm

  if [[ -f ${state_file} ]]; then
    print_error "state file exists, please run 'destroy.sh --all' or remove it manually: 'rm -f ${state_file}'"
  fi

  select_provider
  select_node_count
  select_disk_count

  set_state_var "CLOUD_PROVIDER" "${cloud_provider}"
  set_state_var "NODE_CNT" "${node_cnt}"
  set_state_var "DISK_CNT" "${disk_cnt}"

  deploy_k8s
  sleep 10
  deploy_ais
  ;;
--ais)
  check_command kubectl
  check_command helm

  deploy_ais
  ;;
--dashboard)
  deploy_dashboard
  wait # Wait indefinitely for `kubectl proxy`.
  ;;
--help)
  print_help
  ;;
*)
  print_error "unknown argument provided"
  ;;
esac
