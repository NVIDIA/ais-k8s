#!/bin/bash

# NOTE: Currently, we only have integration tests that run on an existing K8s cluster.
# `USE_EXISTING_CLUSTER=true` is set while running the tests to ensure, `envtest` environement isn't used.

envtest_assets_dir="/tmp/ais-k8s-operator/testbin"
current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

mkdir -p ${envtest_assets_dir}
test -f ${envtest_assets_dir}/setup-envtest.sh || curl -sSLo ${envtest_assets_dir}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh

source ${envtest_assets_dir}/setup-envtest.sh

fetch_envtest_tools ${envtest_assets_dir}
setup_envtest_env ${envtest_assets_dir}

short=""
if [[ $1 == "short" ]]; then
  short="1"
fi

SHORT=${short} TEST_ALLOW_SHARED_NO_DISKS="${TEST_ALLOW_SHARED_NO_DISKS}" TEST_STORAGECLASS="${TEST_STORAGECLASS}" USE_EXISTING_CLUSTER=true ginkgo -v -progress -trace $current_dir/../... -coverprofile cover.out
