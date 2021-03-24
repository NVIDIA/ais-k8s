// Package cmn provides utilities for common AIS cluster resources
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package cmn

const livenessSh = `
 #!/bin/bash
 
 health_url="http://localhost:${CLUSTERIP_PROXY_SERVICE_PORT}/v1/health"

 stat=$(curl -X GET -o /dev/null --max-time 5 --silent -w "%{http_code}" "${health_url}")
 if [[ ${stat} == "200" ]]; then 
	exit 0
 fi

 # If .ais.shutdown marker is present, the node has gracefully shutdown.
 # Kubernetes shouldn't try to restart the pod in this case, so we exit with status code 0.
 [[ -f /var/ais_config/.ais.shutdown ]] && exit 0
 exit 1
 `
