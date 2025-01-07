#!/bin/bash

OPERATOR_DIR="$(cd "$(dirname "$0")/../"; pwd -P)"
# This script is used by Makefile to run commands.
source ${OPERATOR_DIR}/scripts/utils.sh

case $1 in
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

*)
  echo "unsupported argument $1"
  exit 1
  ;;
esac
