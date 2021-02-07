// Package proxy contains k8s resources required for deploying AIS proxy daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */

package proxy

const initProxySh = `
#!/bin/bash
#
envfile="/var/ais_env/env"
rm -f $envfile

#
# Update configuration file,substitute environment variables.
# Environment variables are passed to the init container while creating the proxy statefulset.
#

pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.cluster.local"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}
conf_template="/var/ais_config_template/ais.json"
conf_file="/var/ais_config/ais.json"
envsubst < ${conf_template} > ${conf_file}

if [[ "${MY_POD}" == "${AIS_DEFAULT_PRIMARY}" ]]; then
	echo "export AIS_IS_PRIMARY=true" > $envfile
fi
`
