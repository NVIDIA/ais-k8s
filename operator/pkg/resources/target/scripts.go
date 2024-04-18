// Package target contains k8s resources required for deploying AIS target daemons
/*
 * Copyright (c) 2021, NVIDIA CORPORATION. All rights reserved.
 */
package target

const initTargetSh = `
#!/bin/bash
#

#
# Update configuration file,substitute environment variables.
# Environment variables are passed to the init container while creating the target statefulset.
#


# Obtain the external IP address of the LoadBalancer services associated with the target pod.
if [[ "${ENABLE_EXTERNAL_ACCESS}" == "true" ]]; then
    envfile="/var/ais_env/env"
    external_ip=""
    while [[ -z ${external_ip} ]]; do
        echo "Fetching external IP for service ${MY_POD}"
        external_ip=$(kubectl get services --namespace ${K8S_NS} ${MY_POD} --output jsonpath='{.status.loadBalancer.ingress[0].ip}')
        [[ -z ${external_ip} ]] && sleep 10
    done
fi

if [[ -z ${external_ip} && -n ${AIS_PUBLIC_HOSTNAME} ]]; then
    external_ip=${AIS_PUBLIC_HOSTNAME}
fi

export AIS_PUBLIC_HOSTNAME="${external_ip}"

cluster_domain=${AIS_K8S_CLUSTER_DOMAIN:-"cluster.local"}
pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.${cluster_domain}"

# Check if HOST_NETWORK is true and adjust AIS_INTRA_HOSTNAME and AIS_DATA_HOSTNAME accordingly
if [[ "${HOST_NETWORK:-false}" == "true" ]]; then
    export AIS_INTRA_HOSTNAME=""
    export AIS_DATA_HOSTNAME=""
else
    export AIS_INTRA_HOSTNAME=${pod_dns}
    export AIS_DATA_HOSTNAME=${pod_dns}
fi

# Run script to replace AIS_PUBLIC_HOSTNAME with its entry in the hostname config map if provided
source "/var/global_config/hostname_lookup.sh"

local_conf_template="/var/ais_config_template/ais_local.json"
local_conf_file="/var/ais_config/ais_local.json"
envsubst < ${local_conf_template} > ${local_conf_file}
`
