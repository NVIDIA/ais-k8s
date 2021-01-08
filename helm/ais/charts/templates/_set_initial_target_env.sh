{{- define "target.set_initial_target_env" -}}
#!/bin/bash
#

# TODO: Install in docker image
apk add gettext

pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.cluster.local"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}
conf_template="/var/ais_config_template/ais.json"
conf_file="/var/ais_config/ais.json"
envsubst < ${conf_template} > ${conf_file}
{{end}}
