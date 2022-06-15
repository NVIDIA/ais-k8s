#!/bin/bash


OPERATOR_DIR="$(cd "$(dirname "$0")/../"; pwd -P)"
EXTERNAL_SRC_REGEX=".*\(venv\|build\|3rdparty\|dist\|.idea\|.vscode\)/.*"
# This script is used by Makefile to run commands.
source ${OPERATOR_DIR}/scripts/utils.sh

case $1 in
lint)
  echo "Running lint..." >&2
  golangci-lint run $(list_all_go_dirs)
  exit $?
  ;;

fmt)
  err_count=0
  case $2 in
  --fix)
    echo "Running style fixing..." >&2

    gofumpt -s -w ${OPERATOR_DIR}
    ;;
  *)
    echo "Running style check..." >&2

    out=$(gofmt -l -e ${OPERATOR_DIR})

    if [[ -n ${out} ]]; then
      echo ${out} >&2
      exit 1
    fi

    check_gomod
    check_imports
    check_files_headers
    ;;
  esac
  ;;

spell)
  echo "Running spell check..." >&2
  case $2 in
  --fix)
    ${GOPATH}/bin/misspell -w -locale=US ${OPERATOR_DIR}
    ;;
  *)
    ${GOPATH}/bin/misspell -error -locale=US ${OPERATOR_DIR}
    ;;
  esac
  ;;

*)
  echo "unsupported argument $1"
  exit 1
  ;;
esac
