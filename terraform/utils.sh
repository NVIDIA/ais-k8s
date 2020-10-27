print_error() {
  echo "Error: $1."
  exit 1
}

check_number() {
  if ! [[ "$1" =~ ^[0-9]+$ ]] ; then
    print_error "'$1' is not a number"
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

# TODO: For now it must be divisible by 3 because of GKE. But we should think
#  of something better. Also we need a better validation to check if number is
#  greater than 0 etc.
select_node_count() {
  printf "Enter number of nodes (must be divisible by 3): "
  read -r node_cnt
  check_number "${node_cnt}"
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
