#!/bin/bash
set -euo pipefail

ACTION="${1:-}"
BINARY="${2:-}"
LOCALBIN="${3:-./bin}"
PID_FILE="${LOCALBIN}/cloud-provider-kind.pid"
LOG_FILE="${LOCALBIN}/cloud-provider-kind.log"

start() {
  [ -f "${PID_FILE}" ] && kill -0 "$(cat "${PID_FILE}")" 2>/dev/null && \
    echo "cloud-provider-kind already running" && return 0
  
  "${BINARY}" > "${LOG_FILE}" 2>&1 &
  echo $! > "${PID_FILE}"
  echo "Started cloud-provider-kind"
}

stop() {
  [ ! -f "${PID_FILE}" ] && echo "cloud-provider-kind not running" && return 0
  
  kill "$(cat "${PID_FILE}")" 2>/dev/null || true
  rm -f "${PID_FILE}" "${LOG_FILE}"
  echo "Stopped cloud-provider-kind"
}

case "${ACTION}" in
  start|stop) "$ACTION" ;;
  *) echo "Usage: $0 {start|stop} <binary> [localbin]" >&2; exit 1 ;;
esac

