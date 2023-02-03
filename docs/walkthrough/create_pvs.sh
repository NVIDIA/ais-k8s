#!/usr/bin/env bash

# Part of the AIstore k8s deployent walkthrough.
# Quickly set up hostPath persistent volumes for an aistore deployment.
#
# To use this script, run, for example:
#
#   bash create_pvs.sh --node-count 3 --drives sdb,sdc,sdd --size 10Gi

NS=ais
WORKDIR=/tmp/aisvols
[ ! -d $WORKDIR ] && mkdir $WORKDIR
cd $WORKDIR

usage() {
  ME=$( basename $0 )
  cat <<EOF
$ME

Create kubernetes persistent hostPath volumes for consumption by the
aistore operator.

Usage:

  $ME --node-count <node_count> --drives <drives> [--size <size>]

Where:

  node_count: The number of storage nodes in your cluster.
  drives: Drive names (eg, "sdb) to be used, seperated by commas.
  size: Kubernetes PV size specifier, for example "10Ti". This defaults to "10Gi".
        You should use SI units here - for example, Ei, Pi, Ti, Gi, Mi, Ki.

Both --node-count and --drives are required; it's assumed that
your drives are already mounted, with filesystems, under /ais.

For example, if you specified --paths sdb, a hostPath volume would be
created for the directory /ais/sdb on every node.
EOF
}

options=$(getopt -o '' --long node-count:,drives:,size: -- "$@")
[ $? -eq 0 ] || {
    usage
    exit 1
}
SIZE="10Gi"
eval set -- "$options"
while true; do
    case "$1" in
    --node-count)
        NODES=$2
        shift
        ;;
    --drives)
        DRIVES=$2
        shift
        ;;
    --size)
        SIZE=$2
        shift
        ;;
    --)
        shift
        break
        ;;
    esac
    shift
done

[ -z "${NODES}" ] && usage && exit 1
[ -z "${DRIVES}" ] && usage && exit 1
[ -z "${SIZE}" ] && usage && exit 1
( ! echo $SIZE | egrep '[0-9]+[EPTGMKB]i?$' >/dev/null ) && usage && exit 1

cat <<EOF > ${WORKDIR}/template.yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: aistore-_DRIVE_-target-_INSTANCE_-pv
  labels:
    type: local
spec:
  storageClassName:
  capacity:
    storage: "${SIZE}"
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: "/ais/_DRIVE_"
EOF

for DRIVE in $( echo ${DRIVES} | sed 's/,/\n/g' ); do
  [ ! -d /ais/${DRIVE} ] && echo "Path /ais/$DRIVE not found. Create your filesystems first!" 1>&2 && exit 1
  for INSTANCE in $( seq 0 $(( ${NODES} - 1 )) ); do
    PV_MANIFEST=${WORKDIR}/aistore_${DRIVE}_target_${INSTANCE}_pv.yaml
    cp ${WORKDIR}/template.yaml ${PV_MANIFEST}
    sed -i'' "s/_DRIVE_/${DRIVE}/g" ${PV_MANIFEST}
    sed -i'' "s/_INSTANCE_/${INSTANCE}/g" ${PV_MANIFEST}
    echo "Generated ${PV_MANIFEST}, applying it."
    kubectl -n $NS apply -f ${PV_MANIFEST}
  done
done
