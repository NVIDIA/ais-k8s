// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

const (
	livenessSh = `
	#!/bin/bash

	url_scheme="http"
	if [[ "${AIS_USE_HTTPS}" == "true" ]]; then
		url_scheme="https"
	fi

	health_url="${url_scheme}://localhost:${AIS_NODE_SERVICE_PORT}/v1/health"

	stat=$(curl -X GET -o /dev/null --max-time 5 -k --silent -w "%{http_code}" "${health_url}")
	if [[ ${stat} == "200" ]]; then
		exit 0
	fi

	# If .ais.shutdown marker is present, the node has gracefully shutdown.
	# Kubernetes shouldn't try to restart the pod in this case, so we exit with status code 0.
	[[ -f /var/ais_config/.ais.shutdown ]] && exit 0
	exit 1
 `
	readinessSh = `
	#!/bin/bash
	url_scheme="http"
	if [[ "${AIS_USE_HTTPS}" == "true" ]]; then
		url_scheme="https"
	fi

	source /var/ais_env/env || true
	health_url="${url_scheme}://${CLUSTERIP_PROXY_SERVICE_HOSTNAME}:${CLUSTERIP_PROXY_SERVICE_PORT}/v1/health"
	our_health_url="${url_scheme}://localhost:${CLUSTERIP_PROXY_SERVICE_PORT}/v1/health?readiness=true"

	#
	# If nothing answers with an smap on the clusterIP service then we're in early deployment
	# of a new cluster. If we assert not ready on the initial primary then other nodes can't
	# contact us on the clusterIP service and it just slows initial cluster establishment.
	# So for this early bootstrap phase we always indicate "ready".
	#
	stat=$(curl -X GET -o /dev/null --max-time 5 -k --silent -w "%{http_code}" "${health_url}")
	if [[ "${stat}" != "200" ]]; then
		# Looks like early deployment; make a special case for the initial primary
		[[ "${AIS_IS_PRIMARY}" == "true" ]] && exit 0
	fi

	# otherwise tell the truth for this pod
	stat=$(curl -X GET -o /dev/null --max-time 5 -k --silent -w "%{http_code}" "${our_health_url}")
	[[ "${stat}" == "200" ]] && exit 0
	exit 1
 `
	hostnameMapSh = `
	#!/bin/bash
	# Lookup the hostnames in the hostname config map (allows for multiple host ips)
	hostname_map="/var/global_config/hostname_map"
	if [ -f "$hostname_map" ]; then
		read -ra pairs <<< "$(cat "$hostname_map")"

		for pair in "${pairs[@]}"; do
			IFS='=' read -ra parts <<< "$pair"
			key="${parts[0]}"
			value="${parts[1]}"
			
			if [ "$key" = "$AIS_PUBLIC_HOSTNAME" ]; then
				echo "Setting AIS_PUBLIC_HOSTNAME to value from configMap: ${value}"
				export AIS_PUBLIC_HOSTNAME="$value"
				break
			fi
		done
	fi
`
)
