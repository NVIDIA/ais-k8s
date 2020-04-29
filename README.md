# AIS Helm Chart

This repository includes the Helm chart for deploying AIStore on k8s. It is split from the
[main AIStore repository](https://github.com/NVIDIA/aistore)
to facilitate GitOps style deployment without development noise, although we also supply a
sample shell wrapper to `helm install` if you wish to use Helm directly.

## Can I just `helm install` to try a quick deployment of AIStore?

The main repository includes support for deployment in a standalone Docker container, allowing small scale AIStore deployment
using storage passed using `-v` in `docker run`. To deploy on a multiple-node k8s cluster, however, there's a bit
more preparation required in terms of host tuning and preparation/identification of storage.
The [deployment documentation](docs/README.md) has more detail.

## Deployment Scenarios

### Baremetal On-premises

This has been our reference deployment, and the chart and supporting configuration playbooks show this heritage:
- all k8s nodes are baremetal (not on VMs); the chart doesn't really care whether nodes are baremetal or VMs, but we choose baremetal for straightforward performance
- storage is identified through hostPath mounts, for example each storage node may provide 10 disks with premade filesystems at mountpoints `/ais/sd[a-j]` and those paths are passed to the storage pods for AIStore object storage; the chart does not use PV/PVC for identifying AIS storage - if deploying in cloud you can use PV/PVC and pass those on as hostPath mounts, but we could do to transition to using PVC in all cases.
- the defaults in [supporting playbooks](https://github.com/NVIDIA/aistore/deploy/prod/k8s/playbooks) for host configuration in `vars.yaml` suit our reference systems, eg ethernet device names and NIC types; these tweaks aren't required for deployment, they're just to optimize performance
- our storage nodes have 10 x 10TB HDD each `sd[a-j]` and the supporting playbooks want to tweak the IO scheduler accordingly; those won't apply if you're using SSD/NVME (AIStore works fine with SSD/NVME, but who wants to buy 2PB of such storage?!)
- our deployment is in a "trusted" environment and we run privileged containers, haven't trimmed the ClusterRole permissions to the minimal set, haven't worried too much about access to the AIS administrative REST interface etc!
- where practical we deploy GPU nodes within the same k8s cluster, and use the AIStore proxy clusterIP service DNS name for training URLs - sidestepping questions regarding external ingress
- where we do require external access to an AIStore cluster we don't have the luxury of cloud-provider LoadBalancer services in our baremetal deployment, and so we employ `metallb` for such ingress.

### Baremetal In-Cloud

This is easy - simply emulate baremetal on-premises and employ a cloud LoadBalancer service in front of the AIStore proxy set.

### Cloud

Whether you are running baremetal or VM instances, most of the deployment support (playbooks, Helm) still applies unchanged.
The biggest single tweak required is in sourcing AIStore storage and identifying it to AIStore in Helm. From the AIStore container point of view all it requires is some ready-made filesystems at specified mount paths within the container, and and assurance that each target pod in the target DaemonSet is always provided with exactly the same set of disks.
The chart today uses hostPath volumes, and we can continue to use those in a cloud setting with some preparation in advance. For example:
- if the instance type used for each target node has local HDD (e.g., AWS `d2.8xlarge` instance type with 24 x 2000GB HDD) then just mount those (mkfs with XFS beforehand) at the same set of mountpaths on each target node and we're done
- to use block storage, e.g. AWS EBS, build each proposed target node with the same number of EBS devices per node and as before `mkfs` and mount them at the same set of paths on each node
Ideally the chart would support using a set of pre-established k8s PVC on each proposed target node, and we can satisfy that PVC in any way we require from available resources.

### AIStore as Local File Cache

In addition to serving as a large multinode object store (perhaps itself hydrated from datasets in cloud), AIStore can operate on a smaller scale as a localized cache.
At one extreme AIStore may be used to serve say a 2PB training dataset from centralized storage, and GPU nodes train directly from that - they don't have sufficient local storage to host the dataset, and DL data access patterns of randomized access to the entire dataset in repeated training epochs means there's little to gained from caching even what little fraction you can.
On the other hand, centralized AIStore (or other object store, including cloud) may hold many distinct datasets with some of them being small enough to hold in local storage - say 1TB each. In multinode training it is cumbersome to distribute copies of such smaller datasets to all nodes in advance. AIStore can run locally on such nodes to act as a local file cache, faulting objects into local storage from centralized storage as they are referenced and intelligently pre-fetching the designated bucket ahead of training demand.

