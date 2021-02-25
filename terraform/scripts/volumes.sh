#!/bin/bash

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source $CURRENT_DIR/utils.sh

pv_dir="$state_dir/pv"
pvc_dir="$state_dir/pvc"

locally_persist_volumes() {
  cloud_provider=$(get_state_var "CLOUD_PROVIDER")
  if [[ $cloud_provider != "gcp" ]] ; then
    print_error "unsupported provider"
  fi

  rm -rf $pv_dir
  rm -rf $pvc_dir

  mkdir -p $pv_dir
  mkdir -p $pvc_dir

  for pvc in $(kubectl get pvc -o name); do
    export PV_NAME=$(kubectl get $pvc -o jsonpath="{.spec.volumeName}")
    export PVC_NAME=$(kubectl get $pvc -o jsonpath="{.metadata.name}")
    export PD_NAME=$(kubectl get pv $PV_NAME -o jsonpath="{.spec.gcePersistentDisk.pdName}")

    envsubst < "k8s/${cloud_provider}/templates/pv.yaml" > "${pv_dir}/${PV_NAME}.yaml"
    envsubst < "k8s/${cloud_provider}/templates/pvc.yaml" > "${pvc_dir}/${PVC_NAME}.yaml"
  done
}

restore_persisted_volumes() {
  # Recreate previous PVs.
  (test -d $pv_dir && find $pv_dir -type f -name "*.yaml" -exec kubectl apply -f {} ";") || true
  # Only then recreate PVCs.
  (test -d $pvc_dir && find $pvc_dir -type f -name "*.yaml" -exec kubectl apply -f {} ";") || true
}

clear_persisted_volumes() {
  rm -rf $pv_dir $pvc_dir
}