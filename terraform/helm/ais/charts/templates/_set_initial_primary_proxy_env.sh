{{- define "proxy.set_initial_primary_proxy_env" -}}
#!/bin/bash
#
# Run in an initContainer on proxy pods. Does nothing if the cluster is already
# established (to avoid requiring a node labeled as initial primary in a running
# cluster)
#
# During initial cluster deployment, proxy pods wait here until exactly one node is
# labeled to host the initial primary proxy. If this pod is a proxy running on that
# chosen node then we pass a hint to the main container to startup as primary proxy.
#

envfile="/var/ais_env/env"
rm -f $envfile

#
# On an established cluster we must not depend on the initial primary proxy hack.
# We recognize an established cluster as one for which we can retrieve an smap
# ping from *any* proxy behind the proxy clusterIP service.
#
url="http://${CLUSTERIP_PROXY_SERVICE_HOSTNAME}:${CLUSTERIP_PROXY_SERVICE_PORT}/v1/daemon?what=smap"
echo "Checking for a 200 result on ${url}"
elapsed=0
while [[ $elapsed -lt 5 ]]; do
    code=$(curl -X GET -o /dev/null --silent -w "%{http_code}" $url)
    if [[ "$code" == "200" ]]; then
        echo "   ... success after ${elapsed}s; this is not initial cluster deployment"
        exit 0
    else
        echo "   ... failed ($code) at ${elapsed}s, trying for up to 5s"
        elapsed=$((elapsed + 1))
        sleep 1
    fi
done

#
# Most likely initial cluster deployment time, or a very sick cluster in which no
# proxy could answer on the clusterIP service.  We'll look for a suitably labeled
# node, and if it is the current node then we'll label the current pod as
# initial primary proxy making it inclined to assume the primary role on startup
# unless on startup it discovers otherwise.
#
# Note that initial cluster deployment even the initial primary proxy will "waste"
# a total of 20s above - 10s in the ping and 10s in curl loop. We could shrink that
# but it's a once-off price to pay, and we don't want to startup as tentative
# primary too easily.
#

filter="{{ .Values.proxy.initialPrimaryProxyNodeLabel.name }}={{ template "ais.fullname" . }}"

if [[ "$MY_POD" == "$DEFAULT_PRIMARY_POD" ]]; then
    echo "initContainer complete - this proxy pod is on the primary node $MY_POD"
    #
    # Indicate to subsequent containers in this pod that we started on the primary node.
    #
    echo "export AIS_IS_PRIMARY=true" > $envfile
else
    echo "initContainer complete - not running on primary proxy node"
fi

# Use DNS as hostnames
pod_dns="${MY_POD}.${MY_SERVICE}.${K8S_NS}.svc.cluster.local"
export AIS_INTRA_HOSTNAME=${pod_dns}
export AIS_DATA_HOSTNAME=${pod_dns}

#
# Update configuration file,substitute environment variables
#

global_conf_template="/var/ais_config_template/ais.json"
global_conf_file="/var/ais_config/ais.json"
cp ${global_conf_template} ${global_conf_file}
local_conf_template="/var/ais_config_template/ais_local.json"
local_conf_file="/var/ais_config/ais_local.json"
envsubst < ${local_conf_template} > ${local_conf_file}

{{end}}
