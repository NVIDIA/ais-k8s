#!/bin/bash

# NOTE: Currently, we only have integration tests that run on an existing K8s cluster.
# `USE_EXISTING_CLUSTER=true` is set while running the tests to ensure, `envtest` environment isn't used.

envtest_assets_dir="/tmp/ais-k8s-operator/testbin"
current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

mkdir -p ${envtest_assets_dir}
test -f ${envtest_assets_dir}/setup-envtest.sh || curl -sSLo ${envtest_assets_dir}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh

source ${envtest_assets_dir}/setup-envtest.sh

fetch_envtest_tools ${envtest_assets_dir}
setup_envtest_env ${envtest_assets_dir}

short=""
long=""
if [[ $1 == "short" ]]; then
  LABELS="short && !long"
elif [[ $1 == "long" ]]; then
  LABELS="!short && long"
elif [[ $1 == "manual" ]]; then
  LABELS="!short && !long && override"
else
  LABELS="short || long"
fi 

TEST_STORAGECLASS="${TEST_STORAGECLASS}" USE_EXISTING_CLUSTER=true ginkgo -vv --label-filter="${LABELS}" -trace $current_dir/../... -coverprofile cover.out
