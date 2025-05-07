#!/bin/bash

current_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

# Run e2e tests on existing K8s cluster.
# `USE_EXISTING_CLUSTER=true` is set while running the tests to ensure, `envtest` environment isn't used.

[[ $(command -v ginkgo) ]] || go install github.com/onsi/ginkgo/v2/ginkgo

if [[ $1 == "short" ]]; then
  LABELS="short && !long"
elif [[ $1 == "long" ]]; then
  LABELS="!short && long"
elif [[ $1 == "manual" ]]; then
  LABELS="!short && !long && override"
else
  LABELS="short || long"
fi 

# Run as many workers as the number of tests or twice the CPU core count, whichever is smaller
SPEC_COUNT=$(ginkgo --dry-run --no-color --label-filter="$LABELS" "$current_dir/../tests/e2e/..." 2>&1 | awk '/Will run/{print $3;exit}')
CPU_COUNT=$(nproc)
WORKERS=$(( SPEC_COUNT < CPU_COUNT * 2 ? SPEC_COUNT : CPU_COUNT * 2 ))
[[ -z "$WORKERS" || "$WORKERS" -lt 1 ]] && WORKERS=1

TEST_STORAGECLASS="${TEST_STORAGECLASS}" USE_EXISTING_CLUSTER=true \
  ginkgo -vv -p --procs "$WORKERS" --label-filter="${LABELS}" -trace -coverprofile cover.out $current_dir/../tests/e2e/...
