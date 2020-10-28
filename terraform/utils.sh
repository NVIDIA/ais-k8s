print_error() {
  echo "Error: $1."
  exit 1
}

check_number() {
  if ! [[ "$1" =~ ^[0-9]+$ ]] ; then
    print_error "'$1' is not a number"
  fi
  if (( $1 <= 0 )); then
    print_error "'$1' should be greater than 0"
  fi
}

check_command() {
  if [[ -z $(command -v "$1") ]]; then
    print_error "command '$1' not available"
  fi
}

select_provider() {
  printf "Select cloud provider (aws, azure, gcp): "
  read -r cloud_provider
  if ! [[ ${cloud_provider} =~ ^(aws|azure|gcp)$ ]]; then
    print_error "invalid provider specified"
  fi
}

# TODO: For now it must be divisible by 3 because of GKE - we should think of something better.
select_node_count() {
  printf "Enter number of nodes (must be divisible by 3): "
  read -r node_cnt
  check_number "${node_cnt}"
  if (( node_cnt % 3 != 0 )); then
    print_error "'$node_cnt' is not divisible by 3"
  fi

  node_cnt=$((node_cnt / 3))
}

select_disk_count() {
  printf "Enter number of disk for each target: "
  read -r disk_cnt
  check_number "${disk_cnt}"
}

remove_nodes_labels() {
  kubectl label nodes --all \
    nvidia.com/ais-admin- \
    nvidia.com/ais-target- \
    nvidia.com/ais-proxy- \
    nvidia.com/ais-initial-primary-proxy- \
    1>/dev/null
}

state_file=".deploy.state"

get_state_var() {
  cat ${state_file} 2>/dev/null | grep -w "$1" | cut -d'=' -f2
}

set_state_var() {
  echo "$1=$2" >> ${state_file}
}

unset_state_var() {
  sed -i.bak "/^$1=/d" ${state_file}
}

remove_state_file() {
  rm -f "${state_file}"
}
