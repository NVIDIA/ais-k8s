#!/bin/bash

OPERATOR_DIR="$(cd "$(dirname "$0")/../"; pwd -P)"
# This script is used by Makefile to run commands.
source ${OPERATOR_DIR}/scripts/utils.sh

case $1 in
lint)
  case $2 in
  --fix)
    echo "Running lint with --fix" >&2
    golangci-lint --timeout=15m run $(list_all_go_dirs) --fix
    exit $?
    ;;
  *)
    echo "Running lint..." >&2
    golangci-lint --timeout=15m run $(list_all_go_dirs)
    exit $?
    ;;
  esac
  ;;

fmt)
  case $2 in
  --fix)
    echo "Running style fixing..." >&2

    gofmt -s -w ${OPERATOR_DIR}
    ;;
  *)
    echo "Running style check..." >&2

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
