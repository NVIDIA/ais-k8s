{{- define "target.set_initial_target_env" -}}
#!/bin/bash
#

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
