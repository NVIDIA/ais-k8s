#!/bin/bash
set -e

readonly BOLD='\033[1m'
readonly RED='\033[31m'
readonly RESET='\033[0m'

exec </dev/tty >/dev/tty 2>&1

context=$(kubectl config current-context 2>/dev/null || echo -e "${RED}unset${RESET}")
env="${1}"

echo ""
echo -e "${BOLD}Chart:${RESET}        $(basename "$(pwd)")"
echo -e "${BOLD}Environment:${RESET}  ${env}"
echo -e "${BOLD}Kube Context:${RESET} ${context}"
echo ""

read -p "$(echo -e "${BOLD}Proceed? (Y/N):${RESET} ")" -n 1 -r
echo ""

[[ $REPLY =~ ^[Yy]$ ]] || exit 1
