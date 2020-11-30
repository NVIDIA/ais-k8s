# Planning an AIS Deployment

It helps to have a clear picture of the configuration we are aiming for. Read through and complete the following (in the tiny amount of space provided!)

## Initial Information

Item | Description | Value 
---- | ----------- | -----
Release name | Name used in `helm install` or CD equivalent - becomes base for service names etc; must be DNS compliant. Referenced as `${RELEASE_NAME}` below | _______________
K8s namespace | Namespace for deployment. A cluster can host more than one AIStore cluster, but they must be in distinct namespaces | _______________
AIStore state path prefix | Directory on each proxy/target node under which AIStore state will be persisted; mounted into each Pod via a hostPath volume. Default `/etc/ais` - receommended to keep. | `/etc/ais`

## Container Images

We provide prebuilt container images on GitHub, which will be updated
for major releases.
You will have to update the tag value to track ongoing updates after your initial deployment.

The chart defaults (see `values.yaml`) point to the following:
Item | Default
---- | -------
InitContainer image | gmaltby/ais-kubectl:1
Aisnode image | gmaltby/aisnode:20200521.1

Alternatively, build your own container images, as detailed [in the main repo](https://github.com/NVIDIA/aistore/tree/master/deploy/prod/k8s/aisnode_container).

Item | Description | Value 
---- | ----------- | -----
initContainer image | Initcontainer image for `ais-kubectl`, e.g., `repo.name/ais/ais-kubectl:1`. You will need to update `values.yaml` to point to this. The initContainer image very rarely changes. | _______________
Aisnode image | Aisnode container image name, as above, e.g., `repo.name/ais/aisnode:20200504`. This will also need to go into `values.yaml` and the tag be updated when you want to update the deployment | _______________

## Target Nodes (Pods)

Target nodes are nominated by node labeling to match the target DaemonSet selectors. You can perform the initial deployment
with all the planned nodes ready-labeled, or start with just one or two and label more as you need.

The chart assumes that all target Pods will have precisely the same set of hostPath volume mounts (for its node).

Item | Description | Value 
---- | ----------- | -----
Initial number of targets | Number of target nodes *at initial helm install*. This is a hint to initial cluster formation, which you will record in `values.yaml` - or just leave it as 0 and let clustering work things out. | _______________
Target nodes | Node names planned as hosting targets. e.g. `cpu{01..12}` | _______________
Mountpoints | Filesystem mount points for AIStore on each target node. These will be made available to the target Pod on that node via hostPath volumes. You need to present ready-made clean filesystems (empty from at least the mount point down at install time, anyway), and *they must be different filesystems, ie separate fsid per mountpoint*. We can use lvm stripes/mirrors etc, but individually mounted disks are easiest and preferred. Example: `/ais/sd[a-j]` for 10 HDD mounted to each target node.
Target node labels | Target nodes are nominated (once filesystems are ready!) by labelling nodes (`kubectl label node`) with a label that includes the release name noted above. The label is `nvidia.com/ais-target=${RELEASE_NAME}-ais` | See left

## Proxy Nodes (Pods)

Proxy Pods, also controlled by a DaemonSet, are nominated by node labeling. Only one proxy Pod is required, but more is advised for HA purposes - at least three if possible. Proxies are extremely lightweight - our standard configuration is to run a proxy Pod on each target node (which might be overkill).

Proxies are usually *electable* (eligible to be the primary), but if running a proxy Pod on a GPU node in the same cluster as
AIStore they can be made unelectable.

For initial AIStore cluster deployment *only* one of the electable proxy nodes must also be labeled as the initial primary proxy.
This (horrible hack) is used to bootstrap an initial primary at cluster establishment.

Item | Description | Value 
---- | ----------- | -----
Proxy nodes | Nodes to run proxy Pods, as above. e.g., `cpu{01..12}`. | _______________
Initial primary node | Proxy node on which to bootstrap initial primary at cluster establishment. *Must* also be a proxy node, e.g, `cpu01` | _______________
Proxy node labels | Label all nodes as `nvidia.com/ais-proxy=${RELEASE_NAME}-ais-electable` | See left
Initial primary label | Label one proxy node as `nvidia.com/ais-initial-primary-proxy=${RELEASE_NAME}-ais`; label can be removed once AIStore is started and first proxy is Ready | See left

## Prometheus, Graphite, Grafana

The Helm chart will also default to deploying Prometheus, Graphite & Grafana into the K8s cluster. It does not add
any dashboards - yet to be added to the chart. Prometheus is used to gather host node performance information,
Graphite to gather stats from AIStore, and Grafana to visualize them.

If you do not want these monitoring components installed (e.g., you already have these setup), then change `values.yaml`
to disable them.

The default `values.yaml` will create a PV and PVC for Graphite and Grafana using a quick and dirty hostPath
volume type - you need to nominate the node and filesystem path in `values.yaml`. Alternatively, you can
create a PV and PVC and pass the PVC as `existingClaim` in the Graphite and Grafana section of `values.yaml` - the
comment there will guide you.
