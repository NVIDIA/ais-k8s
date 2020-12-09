# AIStore K8s Deployment Guide

As a high-performance and stateful storage application, deploying AIStore on Kubernetes
requires some planning and preparation. This guide will take you through the planning and
deployment steps.

## Approach

- AIStore doesn't *require* K8s for deployment, but leveraging the K8s platform abstractions makes large multinode deployments a great deal easier; we use a Helm chart for this.
- We assume some degree of K8s expertise; not too much is required for simple deployment scenarios, but if you have a very complex scenario in which to deploy AIStore you'll need correspondingly more expertise.
- We'll start with the simplest scenario in which GPU node clients are in the same K8s cluster; this avoids questions of cluster ingress, loadbalancer, hostPorts, firewalls, which will be covered in an appendix.
- By default, the storage Pods use hostPath volumes for object storage; however, you can change the cluster deployment configuration to use the Kubernetes PV and PVC mechanism.

## Highlevel Overview

You need a K8s cluster in which some number of nodes (we call them storage *target nodes*)
having persistent storage attached or assigned. Any K8s cluster will do as well any storage
type that can present a filesystem for hostPath volumes - but clearly mileage varies, so
we'll link to some discussion of that aspect below.

The core of the deployment consists of two DaemonSets. The first DaemonSet implements the AIStore gateway/proxy
service. A client that wants to GET or PUT an object contacts this service (port 51080 by default)
and is redirected (HTTP redirect) to a specific target Pod for the data transfer (port 51081 by default).
All proxy Pods in the DaemonSet (can be just one, but for HA you'd want 3 or more) provide endpoints for the clusterIP proxy service.
If all GPU nodes are in-cluster then they can use this clusterIP by the K8s DNS name.
If we have external clients then, as usual, we need to configure some ingress to the proxy service.

The *target* Pods deploy as a StatefulSet - these are Pods with access to storage. We pass
readymade filesystems to these target Pods, and have no requirements on the filesystem type
(but we test only XFS!) or on the underlying storage (local HDD, local SSD/NVMe, iSCSI, EBS, ...).
The Helm chart uses hostPath mounts to make the filesystem storage available to target Pods. If you wish to use PV/PVC instead of hostPath,
you should modify the configuration accordingly.

## Standard Initial Deployment - Overview

"standard"  - A straightforward deployment scenario as above - GPU clients in cluster, etc.<br\>
"initial" - An initial/bootstrap deployment of this AIStore cluster; hereafter, you manage the cluster via rolling upgrade.

There's not much to it:

1. Identify which nodes will host target Pods, i.e., those nodes with storage. Some subset, or all, of those nodes, will host proxy Pods, too.
1. Label nodes with `kubectl label node` to identify proxy and target nodes for this AIStore instance
1. Prepare storage on target Pods: make filesystems if needed, mount them at the same mount path on each target node
1. Decide whether you will build and host your own container images, or use those we prebuild; if your container repo is private then add suitable secrets into the namespace you plan to deploy into.
1. Edit `values.yaml` in the `ais` chart to reflect the choices above
1. Deploy using `helm install` or your choice of continuous deployment poison (we use argoCD)
1. Your new AIStore cluster will operate with reasonable, but not optimized, performance.
Some host node tuning is recommended to maximize performance - you will need this for large clusters
sustaining high loads.

## Standard Initial Deployment - Detail

1. Planning:
   - We provide [a planning worksheet](planning.md) in which to capture and plan salient information - intended storage worker/target nodes, proxy nodes, external access, required node labels, etc.
1. Node labeling; you can do this now or after Helm chart install; the labels are just controlling DaemonSets and StatefulSets.
   - Using `kubectl`, label nodes as per the label keys & values listed in the planning table:
      - the set of target nodes,
      - the set of proxy nodes (usually a subset of the target nodes),
      - in addition, label exactly one proxy node as the initial primary proxy - this is needed only for the initial cluster bootstrap.
1. Prepare storage
   - Make data filesystems if necessary and mount using the same set of mount paths on each target node, ready for hostPath volumes
   - Use an `XFS` filesystem if you can - playbooks [ais_datafs_\*.yml](../playbooks) can assist.
   We use `XFS` with mount options `noatime,nodiratime,logbufs=8,logbsize=256k,largeio,inode64,swalloc,allocsize=131072k`
1. Container images
   - We release container images on GitHub - see the [planning doc](planning.md).
   - If building your own, see [the main AIStore repo](https://github.com/NVIDIA/aistore/tree/master/deploy/prod/k8s/aisnode_container).
1. Edit `helm/ais/charts/value.yaml` to reflect the choices above (including those in the planning table).
   - You only need to visit sections `aiscluster` and a couple of small sections after it (if you don't want our monitoring trio of Prometheus/Graphite/Grafana), all at the top of the file.
   - We recommend editing `values.yaml` rather than using over-rides from the `helm install` CLI or from CD tool.
1. Deploy!
   - If using Helm CLI, `helm install ${RELEASE_NAME}` where the release name matches that chosen in the planning step (and which is built into the expected node label values etc, so don't change it now!).
   - If using the present repository with a continuous deployment tool such as [Argo CD](https://argoproj.github.io/argo-cd/), deploy using that
1. Test and Tune!
   - We'll include information on identifying the AIStore service in the cluster below
   - You can use the `aisloader` chart to generate a synthetic GET and PUT load; this can generate a far more extreme load than GPU jobs will (they have that pesky computation step)
   - If you have high-performance components (fast networking) you'll very likely want to apply some system tuning.
   - At the other extreme, if using HDD spinners they will benefit hugely from performance tuning to the IO scheduler.
   - See the Playbooks section below for the full set of tweaks we apply.

#### Additional Information and Recommendations

- We include some Ansible playbooks, detailed below, to assist with many of the steps above.
- Any K8s deployment can be used, but [here is some discussion](k8s_discussion.md) and a description of our reference environment.
- We use Kubespray to build bare-metal K8s clusters - some detail [here](kubespray/README.md).
- our reference environment uses Ubuntu, and the supporting playbooks for host configuration/tuning likely have a few ubuntu assumptions.
- Our reference environment uses XFS for data filesystems, and we recommend using the same if that choice is available for your storage.
- We strongly recommend setting `aiscluster.k8s.sysctls.somaxconn=100000` in `values.yaml` but this requires a change to `kubelet.env` to permit that sysctl, as described in `values.yaml`. Playbook (ais_host_post_kubespray)[../playbooks/ais_host_post_kubespray.yml] can assist.
- We strongly recommend that your K8s cluster use a large MTU if your network
supports it.  Our reference setup has a physical MTU of 9000, and the Calico CNI uses 8980 (Calico must be at least 20 bytes smaller than physical).

### AIStore Service Endpoint

Clients within the cluster should connect to the clusterIP proxy service (behind which sit all the proxy Pods
providing that service). This is available as the following, which pods in the same cluster can use courtesy of K8s DNS:

    http://${RELEASE_NAME}-ais-proxy.${AIS_NAMESPACE}:51080


WebDataset example will show how to interact with AIStore object stores.

### AIStore Admin CLI

Running `make cli` in the [main AIStore repo](https://github.com/NVIDIA/aistore) will build
the AIS CLI. It is also included in the `aisnode` container image used in K8s deployments.

### Playbooks

We include a few [convenient Ansible playbooks](../playbooks/README.md) to assist
in configuring hosts in preparation for deploying AIStore. None of them are required,
but they help with making and mounting filesystems, worker node performance tuning, etc.

## Appendix - Ingress to AIStore

Documentation to follow. If your GPU nodes are in the same K8s cluster as
the AIStore nodes then their access to AIStore services is trivial - no
need for LoadBalancers, Ingress etc.

Our bare-metal deployment uses metallb as a load balancer and a correpsonding ingress to front the AIStore proxy service. For data requests (PUT, GET) the
proxy performs HTTP redirection, and the client must contact the particular target
node to which it is redirected - so a LoadBalancer is not appropriate for the
targets (or maybe a 1:1 LoadBalancer would work). Our solution is to have the
AIStore containers use a hostPort, and have the proxies redirect to the host
node and port.

In the cloud, with cloud-provider loadbalancers and managed K8s offerings, some
deployment steps will differ in minor ways from the above. The most significant
differences will come in LoadBalancers and Ingress, where external GPU
clients are used. We plan to document those use cases soon.
