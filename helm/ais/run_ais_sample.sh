#!/bin/bash -p

#
# Wrapper for helm install of AIS - alternative to repeating all these
# runes on the cmdline: copy and customize this script *if* you are
# deploying via 'helm install' cmdline.
#
# The preferred deployment method is via the like of ArgoCD, in which
# case you can ignore this script.
#

############# BEGIN: Review customization from this point to the marker below #############
#
# AIS cluster name, k8s namespace (must already exist)
#
AIS_NAME=${AIS_NAME:-demo}
AIS_NAMESPACE=default

#
# Container images - select aisnode version, the ais-init image version rarely changes
#
AISNODE_IMAGE=${AISNODE_IMAGE:-aistore/aisnode:3.4}
AIS_INIT_IMAGE=${AIS_INIT_IMAGE:-aistore/ais-init:latest}
ADMIN_IMAGE=${ADMIN_IMAGE:-aistore/admin:3.4}

#
# *If* the images require a pull secret, then install the pull secret in k8s
# and quote the secret name here (not the secret itself!). Leave as empty
# string for public repos.
#
# Make sure you install the secret into $AIS_NAMESPACE !
#
PULLSECRETNAME=""

#
# Mountpaths in AIS target nodes for use by AIS (as hostPath volumes). You must specify this.
# Target nodes are controlled by node labeling. The ais chart today assumes the same paths
# are used on all nodes - this is a restriction of the chart, not of AIS itself.
#
#MOUNTPATHS='{/ais/sda,/ais/sdb,/ais/sdc,/ais/sdd,/ais/sde,/ais/sdf,/ais/sdg,/ais/sdh,/ais/sdi,/ais/sdj}'
MOUNTPATHS=${MOUNTPATHS:-}

EXTERNAL_VOLUMES_COUNT=${EXTERNAL_VOLUMES_COUNT:-0}
EXTERNAL_VOLUMES=$((EXTERNAL_VOLUMES_COUNT > 0))

if [[ ${EXTERNAL_VOLUMES} ]]; then
  mpaths="$(seq -f "/ais/%g" -s "," 1 $EXTERNAL_VOLUMES_COUNT)"
  MOUNTPATHS="{${mpaths/%,/}}" # Removes `,` at the end of the line (if present).
fi


#
# Grafana & Graphite storage - the chart will create hostName PVs for these.
# Grafana is small (just worksheets etc) so assume they're to come from the
# same node as subdirectories of the same tree. The nodename/basepath/size
# below are used in completing a PV/PVC for use with Graphite and Grafana -
# the chart bundles a local-storage PV which will require some modification
# if provisioning from another source.
#
INSTALL_MONITORING=${INSTALL_MONITORING:-true}
STATS_NODENAME=${STATS_NODENAME:-cpu01}
STATS_BASEPATH="/data"
STATS_SIZE="250Gi"

#
# By default we dedicate AIS nodes to AIS and don't restrict it on CPU/mem - it doesn't
# need much except when performing distributed sorts. If you need to restrict CPU/mem
# resource then use the following, otherwise leave as empty strings.
#
CPU_REQUESTS=""			# eg, 40
CPU_LIMITS=""			# eg 44
MEM_REQUESTS=""			# eg 120Gi
MEM_LIMITS=""			# eg 140Gi

#
# External ingress to cluster - pass the cluster CIDR as used in Kubespray
# and the hostport number that will be opened on target nodes and redirected
# to target pods there. If not opening external ingress (ie access to external
# storage clients) then leave AIS_K8S_CLUSTER_CIDR empty.
#
# This has only been tested using metallb - if using a cloud provider
# LoadBalancer then some work may be required.
#
AIS_K8S_CLUSTER_CIDR=${AIS_K8S_CLUSTER_CIDR:-}     # eg 192.168.0.0/18
AIS_HOST_PORT=51081         # don't change unless really necessary
AIS_GATEWAY_EXTERNAL_IP=${AIS_GATEWAY_EXTERNAL_IP:-}  # must be in metalLB pool range if used

#
# Similarly for ingress to Grafana. We also create a NodePort service
# for Grafana, but the ingress has a stable port number.
#
AIS_GRAFANA_EXTERNAL_IP=""

############# END: Review customization above this point #############

helm version >/dev/null 2>&1
if [[ $? -ne 0 ]]; then
	echo "Helm does not appear to be available" >/dev/stderr
	exit 2
fi


if [[ -z "$MOUNTPATHS" ]]; then
	echo "Please fill MOUNTPATHS" >&2
	exit 2
fi

if $INSTALL_MONITORING; then
	NO_MONITORING="nope"
	if [[ ! -f "charts/requirements.lock" ]]; then
		# pull dependencies automatically just once; first add repo
		(cd charts && helm dependency update)
		if [[ $? -ne 0 ]]; then
			echo "helm dependency update failed!" >/dev/stderr
			exit 2
		fi
	fi
else
	NO_MONITORING=""
fi

helm install \
	--namespace=$AIS_NAMESPACE \
	--set aiscluster.image.pullPolicy=${AIS_PULL_POLICY:-IfNotPresent} \
	--set-string aiscluster.image.aisnode.repository=$(echo $AISNODE_IMAGE | cut -d: -f1) \
	--set-string aiscluster.image.aisnode.tag=$(echo $AISNODE_IMAGE | cut -d: -f2) \
	--set-string aiscluster.image.kubectl.repository=$(echo $AIS_INIT_IMAGE | cut -d: -f1) \
	--set-string aiscluster.image.kubectl.tag=$(echo $AIS_INIT_IMAGE | cut -d: -f2) \
	--set-string admin.image.repository=$(echo $ADMIN_IMAGE | cut -d: -f1) \
	--set-string admin.image.tag=$(echo $ADMIN_IMAGE | cut -d: -f2) \
	${PULLSECRETNAME:+ --set-string aiscluster.image.pullSecretNames="${PULLSECRETNAME}"} \
	--set-string aiscluster.target.mountPaths="${MOUNTPATHS}" \
	--set aiscluster.target.externalVolumes="${EXTERNAL_VOLUMES}" \
	${NO_MONITORING:+ --set-string graphite.ais.pv.node=$STATS_NODENAME} \
	${NO_MONITORING:+ --set-string graphite.ais.pv.path=${STATS_BASEPATH}/graphite} \
	${NO_MONITORING:+ --set-string graphite.ais.pv.capacity=${STATS_SIZE}} \
	${NO_MONITORING:+ --set-string grafana.ais.pv.node=$STATS_NODENAME} \
	${NO_MONITORING:+ --set-string grafana.ais.pv.path=${STATS_BASEPATH}/grafana} \
	${NO_MONITORING:+ --set-string grafana.ais.pv.capacity=${STATS_SIZE}} \
	${CPU_REQUESTS:+ --set-string target.resources.requests.cpu=${CPU_REQUESTS}} \
	${CPU_LIMITS:+ --set-string target.resources.limits.cpu=${CPU_LIMIT}} \
	${MEM_REQUESTS:+ --set-string target.resources.requests.memory=${MEM_REQUESTS}} \
	${MEM_LIMITS:+ --set-string target.resources.limits.memory=${MEM_LIMITS}} \
	${AIS_K8S_CLUSTER_CIDR:+ --set-string aiscluster.k8s.cluster_cidr="${AIS_K8S_CLUSTER_CIDR}"} \
	${AIS_HOST_PORT:+ --set-string aiscluster.target.hostPort=${AIS_HOST_PORT}} \
	${AIS_GATEWAY_EXTERNAL_IP:+ --set-string aiscluster.ingress.gateway.externalIP=${AIS_GATEWAY_EXTERNAL_IP}} \
	${AIS_GRAFANA_EXTERNAL_IP:+ --set-string aiscluster.ingress.grafana.externalIP=${AIS_GRAFANA_EXTERNAL_IP}} \
	${HELM_ARGS} \
	"${AIS_NAME}" \
	charts/.
