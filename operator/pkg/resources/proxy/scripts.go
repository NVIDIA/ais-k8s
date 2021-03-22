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

cluster_domain=${AIS_K8S_CLUSTER_DOMAIN:-"cluster.local"}
pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.${cluster_domain}"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}

local_conf_template="/var/ais_config_template/ais_local.json"
local_conf_file="/var/ais_config/ais_local.json"
envsubst < ${local_conf_template} > ${local_conf_file}

if [[ "${MY_POD}" == "${AIS_DEFAULT_PRIMARY}" ]]; then
	echo "export AIS_IS_PRIMARY=true" > $envfile
fi
`
