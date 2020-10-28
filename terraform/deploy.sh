#!/bin/bash

set -eo pipefail

source utils.sh

deploy_ais() {
  echo "🔥 Deploying AIStore on the cluster"

  # Remove labels (from all nodes) if exist.
  remove_nodes_labels

  # Label nodes so they match a required selector.
  kubectl label nodes --all \
    nvidia.com/ais-target=demo-ais \
    nvidia.com/ais-proxy=demo-ais-electable \
    1>/dev/null

  # Label one of the nodes to mark it as a primary.
  primary_node=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
  kubectl label nodes "${primary_node}" \
    nvidia.com/ais-admin=demo-ais \
    nvidia.com/ais-initial-primary-proxy=demo-ais \
    1>/dev/null

  external_ip=$(terraform output external_ip)
  pushd ../helm/ais > /dev/null

  AIS_GATEWAY_EXTERNAL_IP="${external_ip}" \
    AIS_K8S_CLUSTER_CIDR="10.64.0.0/14" \
    AISNODE_IMAGE="aistore/aisnode-k8s:v7" \
    KUBECTL_IMAGE="gmaltby/ais-kubectl:1" \
    EXTERNAL_VOLUMES_COUNT="${disk_cnt}" \
    STATS_NODENAME="${primary_node}" \
    HELM_ARGS="--set tags.builtin_monitoring=false,tags.prometheus=false,aiscluster.expected_target_nodes=$(kubectl get nodes --no-headers | wc -l | xargs),admin.enabled=true" \
    ./run_ais_sample.sh

  popd > /dev/null

  set_state_var "AIS_DEPLOYED" "true"
}

deploy_k8s() {
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

    username=$(gcloud config get-value account)
    if [[ -z ${username} ]]; then
      print_error "username is not set in 'gcloud'"
    fi

    set_state_var "GKE_PROJECT_ID" "${project_id}"
    set_state_var "GKE_USERNAME" "${username}"

    terraform_args=(-var "project_id=${project_id}" -var "user=${username}" -var "node_count=${node_cnt}")
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
    gcloud container clusters get-credentials "$(terraform output kubernetes_cluster_name)" --region "$(terraform output region)"
    echo "✅ kubectl configured to use '$(kubectl config current-context)' context"
  fi

  pushd k8s/
  echo "Initializing persistent storage"
  terraform init -input=false "${cloud_provider}" 1>/dev/null
  terraform apply -input=false -auto-approve "${cloud_provider}"
  popd

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


case $1 in
--all)
  check_command terraform
  check_command kubectl
  check_command helm

  select_provider
  select_node_count
  select_disk_count

  set_state_var "CLOUD_PROVIDER" "${cloud_provider}"
  set_state_var "NODE_CNT" "${node_cnt}"

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
*)
  print_error "unknown argument provided"
  ;;
esac
