# AIStore K8s Deployment Scenarios

AIStore can be deployed on any K8s cluster (subject to a couple of configuration tweaks to be
able to specify the `net.core.somaxconn` in a container, and to permit privileged containers
if serving external clients via a hostPort). Clearly, however, mileage will vary  for various
choices. We'll discuss a number of those aspects in the following sections. First, a word on
deployment types for various use-cases:
- If you are deploying a large, multinode, multi-petabyte storage cluster to serve many hungry GPU node
clients then you will be well-served by optimizing all of the choices below - performance and scalability are critical
- If you are deploying to serve a small number of researchers with a modest number of nodes and GPUs per node, then AIStore is capable of running on anything from a single node upwards and only requires one storage path per target node; it is also very modest in its resource demands for CPU and memory (the exception being dSort) - so start with a modest config and scale as required
- AIStore can also function as a caching tier, providing "local" object storage for DL datasets held in cloud buckets or in data lake storage; it can intelligently fetch and prefetch bucket objects and cache the entire dataset in the AIStore bucket to avoid repeated WAN traversal or hitting the data lake on each epoch; for such deployments, you can utilize K8s as described here and using storage on the GPU nodes themselves (or anything else that is free and is close by), or it might be simpler to use AIStore from a Docker container directly.

## K8s Deployment Choice wrt AIStore

For each aspect discussed, we'll include information on our "reference" configuration. The reference values are not necessarily carefully optimized - some of them just come with the set of equipment that happened to
be available to us.

### Baremetal vs VM Nodes (and hypervisor choice)

AIStore functionality is unaffected by this choice, but performance and
observability may be. The larger cloud provider instance, e.g., AWS `d2.8xlarge` should be perfectly
adequate for large-scale deployments. If providing your own VMs your mileage may vary! Use whichever
provides the convenience and performance you need, and which can satisfy the additional choices
below.

> Reference: Baremetal K8s

### Node CPU and Memory Resource

AIStore has very modest CPU and memory requirements, except when running a dSort, which will benefit from
additional memory. In regular object storage operation mode, proxy requests are trivially lightweight, and object
data is streamed on HTTP sockets, so it does not demand substantial memory.

Distributed Sort (dSort) applied to large sharded datasets uses a controllable fraction of the memory
available to a target node container, so while it benefits from more memory, it won't exhaust the
allocation.

AIStore only benefits from lots of memory in which to cache object data when serving datasets for which a significant
fraction of the data can be cached in the cumulative total memory of all target nodes. If serving a petascale
dataset and total dram is a small fraction of that then all we require is memory to cache
filesystem metadata, but otherwise the DL data access pattern (repeated full dataset passes in randomly
permuted order each pass) renders dram caching useless.

> Reference: Nodes (each runs 1 target and 1 proxy Pod) have 192GB of memory each; 2 CPU sockets, each with 12 hyperthreded cores for a total of 48 CPUs. AIStore typically used 10-15% of CPU (most is iowait time) and less than 10% of memory (most is useless dram cache); for larger dSorts the target nodes are resource limited in K8s to 140GB each.

### Storage Type and Density

Direct attached storage such as HDD/SSD/NVMe is best for performance, but AIStore will still function
with storage options such as iSCSI volumes, NFS mounts, etc. The fewer layers of transport the better
for both performance and reliability/observability/debug, hence the preference for locally attached storage.

If serving a modest total dataset size, then SSD/NVMe storage is economically practical and would certainly
be the quicker storage choice. For larger dataset sizes, HDDs are by far the more economical choice and when
combined with an aggregated data format such as tar shards can feed a hungry set if GPUs perfectly well.

Configure as many HDDs per target node as a) physical form factor permits, b) available CPU can serve, and c) are approximately matched to your network bandwidth. Our reference configuration has just 10 X 10TB HDD per node
since that is what the server chassis holds and we're not using JBODs; we have adequate CPU for a total
of at least 50 HDD per node; each node connects to the switch via 50 gigabit ethernet, which can achieve say 6GB/s so if we assume roughly 200MB/s/HDD the network could handle ~30 HDDs all streaming at once.

> Reference: 10 x 10TB HDD per node (Seagate Helium ST10000NM0096, SAS) mounted as single disks (no lvm)

### Filesystem Type

AIStore requires one or more filesystems per target node, with the only requirement being extended attribute support. All testing and benchmarking has used XFS, which has served very well; ext4 did not begin to match XFS;
OpenZFS was used on NVMe tests and suffered a particular bottleneck apparently known to occur with such drives
under OpenZFS.

> Reference: XFS, mount options as in playbooks (`noatime,nodiratime,logbufs=8,logbsize=256k,largeio,inode64,swalloc,allocsize=131072k,nobarrier`)

### Networking Bandwidth

Ideally, on target nodes, network bandwidth should be matched to the number of drives times their average streaming speed,
as explained above. If serving smaller (cacheable) datasets then you could still use more bandwidth.
Proxy Pods see lots of small packet traffic - HTTP requests and redirects - but small total volume;
running proxy Pods on the same nodes as targets is not an issue.

We do not configure Pods with multiple interfaces, so intra-cluster traffic (such as rebalance) is transported
over the same interfaces.

Another factor is the network between the GPU clients and the AIStore cluster. No surprise to say
that the rule is "the more the merrier"!

Note that in AIStore a given object GET request is only
ever returned from a single target node, ie unlike say HDFS objects are not striped across nodes
and all nodes respond with some client-side software performing re-assembly. In AIStore, the client
GETs (or PUTs) the object's data over a single streaming HTTP socket. Of course a target node may serve
many such requests in parallel, and a GPU client node can make many requests in parallel, too (e.g.,
8 GPUs per node, PyTorch with 5 dataloaders per GPU => 40 shards being stream at once per client node).
With objects spread across the cluster nodes, it is possible/common for several target nodes to be responding to
any one client node at a time and so be able to saturate its network bandwidth.

> Reference: Target/Proxy nodes have 100 gigabit ethernet Mellanox CX-5 NICs, but connect to the switch
> at 50Gb/s; when using in-cluster GPU nodes they connect to the switch at 100Gb/s.

### K8s CNI Plugin

Any high-performance CNI plugin will suffice. We only configure one interface per Pod today (no cluster-private network
for rebalance and control traffic) so no requirement for Multus. Most testing has been with Calico, with a little on Flannel.

> Reference: Calico

### In-cluster Clients vs External Clients

In-cluster clients are simply easier to manage. For example, they can contact AIStore endpoints
using K8s DNS names for the clusterIP proxy service and no need to configure ingress etc.

While you *can* schedule AIStore Pods onto GPU nodes, those nodes typically need all the CPU
power available to perform data augmentation (40 dataloaders, 5 per GPU on an 8 GPU node -
worse if 16 GPUs per node!). The other way around can work, however, scheduling CPU data
augmentation Pods onto target nodes - see [Tensorcom](https://github.com/NVlabs/tensorcom) for that.

> Reference: when using between 1 and 14 NVIDIA DGX-1 nodes (so 1 to 112 GPUs) in the same racks
> sharing the same TOR switches as the AIStore nodes, GPU nodes were included in the cluster;
> deployments with many more GPU nodes use a distinct cluster, simply because they're owned
> and managed by an independent part of the organization; they're still "close" in terms of
> network topology.

#### LoadBalancer and Ingress

If using external clients then you will require a load balancer and a LoadBalancer ingress service
on the proxy clusterIP service. This is so that clients can contact a single well-known
IP address (or DNS entry for it) when initiating GET and PUT via the proxy. We only require the
ingress to direct traffic to the AIStore proxy clusterIP service - we don't require any actual
load-balancing as Kubeproxy/IPVS will do that for us. With many proxy Pods backing the clusterIP
service this effectively provides an HA proxy endpoint.

For baremetal on-premises deployments we use [metallb](https://metallb.universe.tf/). If running in cloud you can use
the cloud provider loadbalancer services - a standard HTTP loadbalancer will do.

The target Pods respond *directly* to clients, and only the target Pod that the proxy redirects
the client to must respond. There is no equivalent clusterIP service for target Pods, 
so no ingress is required for those. Instead, our solution is for the target Pods to listen
on a hostPort when serving external clients.

> Reference: We use metallb for baremetal on-premises K8s.

### Host Performance Tuning

A few tuning parameters are required to support a high HTTP GET/PUT load - those related to socket counts,
port number, port re-use etc - these are captured in playbooks. Additional tuning is advised if
performance expectations are high - e.g., if high bandwidth networking is available - and if using
HDD then tuning the IO scheduler produces excellent dividends. These additional tuneable are also
captured in playbooks.

> Reference: We apply all the tuning captured in the playbooks
