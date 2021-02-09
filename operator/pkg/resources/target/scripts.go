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

export AIS_PUBLIC_HOSTNAME="${external_ip}"
pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.cluster.local"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}
conf_template="/var/ais_config_template/ais.json"
conf_file="/var/ais_config/ais.json"
envsubst < ${conf_template} > ${conf_file}
`
