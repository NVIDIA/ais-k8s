print_error() {
  echo "Error: $1."
  exit 1
}

check_command() {
  if [[ -z $(command -v "$1") ]]; then
    print_error "command '$1' not available"
  fi
}

check_providers() {
  printf "Select cloud provider (aws, azure, gcp): "
  read -r cloud_provider
  if ! [[ ${cloud_provider} =~ ^(aws|azure|gcp)$ ]]; then
    print_error "invalid provider specified"
  fi
}