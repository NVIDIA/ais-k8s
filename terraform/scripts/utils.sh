release_name="demo"
cluster_name="ais"
k8s_dataplane="kube-proxy"

trap 'echo "Please wait for the script to finish or data loss may occur."' INT

print_error() {
  echo "Error: $1."
  exit 1
}

print_warning() {
  echo "WARNING: $1."
}

check_number() {
  if ! [[ "$1" =~ ^[0-9]+$ ]] ; then
    print_error "$2 '$1' is not a number"
  fi
  if (( $1 <= 0 )); then
    print_error "$2 '$1' should be greater than 0"
  fi
}

check_command() {
  if [[ -z $(command -v "$1") ]]; then
    print_error "command '$1' not available"
  fi
}

select_provider() {
  if [[ -z ${cloud_provider} ]]; then
    printf "Select cloud provider (aws, azure, gcp): "
    read -r cloud_provider
  fi
  if ! [[ ${cloud_provider} =~ ^(aws|azure|gcp)$ ]]; then
    print_error "invalid provider specified: '${cloud_provider}' (expected one of: [aws, azure, gcp])"
  fi
}

select_node_count() {
  if [[ -z ${node_cnt} ]]; then
    printf "Enter number of nodes: "
    read -r node_cnt
  fi
  check_number "${node_cnt}" "node count value"
}

select_disk_count() {
  if [[ -z ${disk_cnt} ]]; then
    printf "Enter number of disk for each target: "
    read -r disk_cnt
  fi
  check_number "${disk_cnt}" "disk count value"
}

validate_cluster_name() {
  if [[ -z ${cluster_name} ]]; then
    print_error "cluster name cannot be empty"
  fi
}

validate_dataplane() {
  if ! [[ ${k8s_dataplane} =~ ^(kube-proxy|cilium)$ ]]; then
     print_error "invalid dataplane specified: '${k8s_dataplane}' (expected one of: [kube-proxy, cilium])"
  fi
}

remove_nodes_labels() {
  kubectl label nodes --all \
    nvidia.com/ais-admin- \
    nvidia.com/ais-target- \
    nvidia.com/ais-proxy- \
    nvidia.com/ais-initial-primary-proxy- \
    1>/dev/null
}

# Returns terraform output value for a provided key. It removes `"` quotes so
# they won't be passed as part of the value.
terraform_output() {
  cloud_provider=$(get_state_var "CLOUD_PROVIDER")
  terraform -chdir="${cloud_provider}" output -json "$1" | xargs
}

state_dir="$(cd "$(dirname "$0")" && pwd)/.state"
state_file="$state_dir/deploy"

init_state_dir() {
  mkdir -p $state_dir
}

get_state_var() {
  cat "${state_file}" 2>/dev/null | grep -w "$1" | cut -d'=' -f2
}

unset_state_var() {
  # NOTE: Cannot use `-i` as it is not portable (see: https://unix.stackexchange.com/a/92907).
  sed -e "/^$1=/d" "${state_file}" > "${state_file}.new"
  mv -- "${state_file}.new" "${state_file}"
}

set_state_var() {
  unset_state_var "$1" 1>/dev/null 2>&1 || true
  echo "$1=$2" >> "${state_file}"
}

remove_state_file() {
  rm -f "${state_file}"
}

set_aws_creds() {
  if [[ -n ${aws_creds_dir} ]]; then
      local_aws=$(mktemp)

      aws_secret_name="aws-credentials"
      temp_file="$aws_creds_dir/credentials"
      if [ -f $"$temp_file" ]; then
          cp $"$temp_file" ${local_aws}
      else
          echo "No AWS credentials file found in specified directory. Exiting..."
          exit 1
      fi

      # By default, the region field is found in the aws config file.
      # Sometimes it is found in the credentials file.
      if [ $(cat "$temp_file" | grep -c "region") -eq 0 ]; then
          temp_file="$aws_creds_dir/config"
          if [ -f $"$temp_file" ] && [ $(cat $"$temp_file" | grep -c "region") -gt 0 ]; then
              grep region "$temp_file" >> ${local_aws}
          else
              echo "No region config field found in aws directory. Exiting..."
              exit 1
          fi
      fi

      if kubectl get secrets | grep aws > /dev/null 2>&1; then
        kubectl delete secret ${aws_secret_name}
      fi
      kubectl create secret generic ${aws_secret_name} --from-file=credentials=$local_aws
      rm ${local_aws}
  fi
}
