{{- define "target.set_initial_target_env" -}}
#!/bin/bash
#

# Obtain the external IP address of the LoadBalancer services associated with the target pod.
if [[ "${MY_SERVICE_TYPE}" == "LoadBalancer" ]]; then 
    envfile="/var/ais_env/env"
    external_ip=""
    while [[ -z ${external_ip} ]]; do
        echo "Fetching external IP for service ${MY_POD}"
        external_ip=$(kubectl get services --namespace ${K8S_NS} ${MY_POD} --output jsonpath='{.status.loadBalancer.ingress[0].ip}')
        [[ -z ${external_ip} ]] && sleep 10
    done
fi

export AIS_PUB_HOSTNAME="${external_ip}"
pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.cluster.local"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}
global_conf_template="/var/ais_config_template/ais.json"
global_conf_file="/var/ais_config/ais.json"
cp ${global_conf_template} ${global_conf_file}
local_conf_template="/var/ais_config_template/ais_local.json"
local_conf_file="/var/ais_config/ais_local.json"
envsubst < ${local_conf_template} > ${local_conf_file}

{{end}}
