#!/bin/bash

# NOTE: Currently, we only have integration tests that run on an existing K8s cluster.
# `USE_EXISTING_CLUSTER=true` is set while running the tests to ensure, `envtest` environment isn't used.

current_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

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
