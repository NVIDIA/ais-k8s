#!/bin/bash

set -eo pipefail

# TODO: It seems like a good idea to remember stuff that user entered so that
#  using `destroy.sh` would be automatic. For example we could remember cloud,
#  project ids, flags as well as created `kubectl` contexts.

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
    MOUNTPATHS="{/tmp}" \
    STATS_NODENAME="${primary_node}" \
    HELM_ARGS="--set tags.builtin_monitoring=false,tags.prometheus=false,aiscluster.expected_target_nodes=$(kubectl get nodes --no-headers | wc -l | xargs),admin.enabled=true" \
    ./run_ais_sample.sh

  popd > /dev/null
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
    terraform_args=(-var "project_id=${project_id}")
  fi

  # Initialize terraform and download necessary plugins.
  terraform init -input=false "${cloud_provider}"

  # Execute terraform plan. The approved automatically as we assume that everything is correct.
  echo "🔥 Starting Kubernetes cluster..."
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

  echo "🔄 Setting up kubectl..."
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

  check_providers

  deploy_k8s
  sleep 10
  deploy_ais

  wait # Wait indefinitely for `kubectl proxy`.
  ;;
--ais)
  check_command kubectl
  check_command helm

  deploy_ais
  ;;
*)
  print_error "unknown argument provided"
  ;;
esac
