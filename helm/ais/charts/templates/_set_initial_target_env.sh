{{- define "target.set_initial_target_env" -}}
#!/bin/bash
#

# TODO: Install in docker image
apk add gettext

# TODO: Add docker image validation and use Pod FQN
export AIS_INTRA_HOSTNAME="$(hostname -i)"
export AIS_DATA_HOSTNAME="$(hostname -i)"
conf_template="/var/ais_config_template/ais.json"
conf_file="/var/ais_config/ais.json"
envsubst < ${conf_template} > ${conf_file}
{{end}}
