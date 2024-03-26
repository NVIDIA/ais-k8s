# AIStore Kubernetes (K8s) Deployment Scenarios

AIStore offers flexible deployment options on Kubernetes (K8s) clusters. The effectiveness of these deployments varies based on the chosen configurations. This document outlines key deployment scenarios and offers guidance for each.

# Contents

1. [**Overview**](#Overview)
   - [Large-Scale Deployments](#Large-Scale-Deployments)
   - [Moderate-Scale Deployments](#Moderate-Scale-Deployments)
   - [Fast-Tier Storage Deployments](#Fast-Tier-Storage-Deployments)

2. [**K8s Deployment Choice wrt AIStore**](#K8s-Deployment-Choice-wrt-AIStore)
   - [Choosing Between Baremetal and VM Nodes](#Choosing-Between-Baremetal-and-VM-Nodes)
   - [Node CPU and Memory Resource](#Node-CPU-and-Memory-Resource)
   - [Storage Type and Density](#Storage-Type-and-Density)
   - [Filesystem Type](#Filesystem-Type)
   - [Optimizing Network Bandwidth](#Optimizing-Network-Bandwidth)
   - [Kubernetes CNI Plugin](#Kubernetes-CNI-Plugin)

3. [**Managing In-Cluster vs. External Clients in AIStore**](#Managing-In-Cluster-vs-External-Clients-in-AIStore)
   - [In-Cluster Clients: Simplified Management](#In-Cluster-Clients-Simplified-Management)
   - [External Clients: Additional Setup Required](#External-Clients-Additional-Setup-Required)
   - [LoadBalancer and Ingress](#LoadBalancer-and-Ingress)
   - [Specific Solutions for Different Environments](#Specific-Solutions-for-Different-Environments)

4. [**Host Performance Tuning**](#Host-Performance-Tuning)
   - [Tuning Parameters and Playbooks](#Tuning-Parameters-and-Playbooks)
   - [Additional Resources for Performance Enhancement](#Additional-Resources-for-Performance-Enhancement)


## Overview
Different deployment types cater to specific use-cases in AIStore's versatile environment. Here, we explore these scenarios:

### Large-Scale Deployments
- **Use-Case**: Ideal for extensive, multi-node, multi-petabyte storage clusters serving numerous GPU node clients.
- **Key Considerations**: Focus on optimizing performance and scalability. Every aspect of the deployment should be fine-tuned to meet the high demands of large-scale operations.

### Moderate-Scale Deployments
- **Use-Case**: Suitable for smaller teams of researchers, with limited nodes and GPU resources.
- **Characteristics**: AIStore efficiently operates on configurations ranging from a single node to multiple nodes. It requires minimal resources in terms of CPU and memory, with the notable exception of dSort.
- **Approach**: Begin with a basic setup and scale according to your evolving requirements.

### Fast-Tier Storage Deployments
- **Use-Case**: Functions as a caching tier for Deep Learning (DL) datasets stored in cloud buckets or data lakes.
- **Capabilities**: AIStore can smartly handle data retrieval and prefetching of bucket objects. It caches entire datasets locally in the AIStore bucket, minimizing the need for repeated Wide Area Network (WAN) traversals or frequent data lake accesses.
- **Deployment Options**: Implement this using K8s, leveraging storage directly on GPU nodes or nearby resources. Alternatively, a simpler approach might be to deploy AIStore within a Docker container.

## K8s Deployment Choice wrt AIStore

This document provides detailed insights and recommendations for deploying AIStore in a Kubernetes environment. Our reference configuration, while not meticulously optimized, is based on the equipment we had available. These guidelines will help you tailor the deployment to your specific requirements.

### Choosing Between Baremetal and VM Nodes

The choice between baremetal and VM nodes doesn't significantly affect AIStore's core functionality. However, it can influence performance and observability. The critical requirement for VMs is the attachment of some disks. The performance generally improves with the addition of more disks.

### Node CPU and Memory Resource

AIStore's demands for CPU and memory resources are generally low, with a few exceptions. Some AIStore extensions, such as ETL (Extract, Transform, Load) processes and data resharding, are more compute-intensive. 

- **Standard Operations**: In normal operations involving object storage, proxy requests are minimal in terms of resource usage. Data streaming is handled via HTTP sockets, which does not require significant memory resources.
  
- **Resharding**: When dealing with large, sharded datasets, resharding processes use a controlled portion of the memory available to a target node's container. While having more memory is advantageous, the process is designed not to deplete the entire memory allocation.

- **ETL Processes**: For ETL operations, a separate ETL pod is initiated for each target to execute data transformations. The intensity of computing resources used depends on the nature of these transformations.

- **Memory and Disk Utilization**: Adequate memory can reduce disk utilization. A larger memory capacity allows for more objects to be stored in [Linux's Page Cache](https://www.thomas-krenn.com/en/wiki/Linux_Page_Cache_Basics). This setup can lead to faster GET requests for these objects, as they can be served directly from memory, thereby reducing disk reads and lowering overall disk utilization. 

  > Based on our benchmarks, the use of page cache didn't result in a significant increase in throughput. However, this might vary, especially with slower disk types. Our testing was conducted using NVMe drives.

### Storage Type and Density

This section focuses on the selection of storage types and their density, crucial for optimizing AIStore deployment.

#### Choice of Storage Type

- **Direct Attached Storage (DAS)**: Options like HDDs, SSDs, and NVMe drives are preferred for performance reasons. Direct attached storage ensures fewer layers of transport, which is beneficial for performance, reliability, observability, and debugging. Hence, we recommend locally attached storage when possible.

- **Networked Storage Options**: AIStore is also compatible with networked storage solutions like iSCSI volumes and NFS mounts. While these options are functional, they may not offer the same level of performance as DAS.

#### Storage Density and Type Based on Dataset Size

- **For Modest Dataset Sizes**: If your total dataset size is relatively small, SSDs or NVMe drives are economically viable and offer superior speed.

- **For Large Dataset Sizes**: HDDs are more cost-effective for larger datasets. Utilizing aggregated data formats, such as tar shards, can efficiently feed a large set of GPUs. 

#### Configuring HDDs per Target Node

When configuring HDDs in your nodes, consider the following:

1. **Physical Form Factor**: The number of HDDs is often limited by the physical capacity of your server chassis. 

2. **CPU Capability**: Ensure that your CPU can efficiently manage the number of HDDs. A more powerful CPU can handle more drives.

3. **Network Constraints and Potential**: The capacity of your network connection is a pivotal factor in determining the efficiency of your data transfers. For example, a 50 Gbps Ethernet link is capable of achieving a maximum transfer speed of up to 6.25 GB/s.

### Filesystem Type

When setting up AIStore, each target node needs to be equipped with one or more filesystems. The primary criterion for these filesystems is the support for extended attributes. 

- **XFS as the Preferred Choice**: In our testing and benchmarking processes, we have predominantly used the XFS filesystem. This choice has proven to be highly effective and reliable in our deployments.

- **Comparison with Other Filesystems**: We have found that ext4 does not perform as efficiently as XFS in our setups. Additionally, while OpenZFS was utilized in tests involving NVMe drives, it encountered a specific bottleneck. This issue seems to be a known limitation when using OpenZFS with NVMe drives.

- **Ease of Setup**: For your convenience, we provide a [playbook](../playbooks/host-config/ais_datafs_mkfs.yml) to assist in configuring the disks appropriately for AIStore.

### Optimizing Network Bandwidth

Effective network bandwidth management is crucial in AIStore environments, especially for target nodes. Here's how to optimize it:

#### Matching Bandwidth with Drive Capacity and Streaming Speed

- **Core Principle**: The network bandwidth on target nodes should ideally be proportional to the cumulative streaming speed of all drives. This ensures that the network can handle the data traffic generated by the drives without bottlenecks.

- **Smaller Datasets and Bandwidth Utilization**: Even with smaller datasets that can be cached, it's beneficial to have higher bandwidth to accommodate potential data traffic spikes.

#### Proxy Pods and Network Traffic

- **Traffic Characteristics**: Proxy Pods primarily handle small packet traffic, which includes HTTP requests and redirects. However, the total volume of this traffic is relatively low.

- **Co-Location with Target Nodes**: Running Proxy Pods on the same nodes as target nodes typically doesn't lead to network congestion, due to the modest volume of traffic generated by these Pods.

#### Network Configuration for Pods

- **Single Interface Configuration**: In our setup, Pods are not configured with multiple interfaces. This means that both intra-cluster traffic (like data rebalancing) and external traffic are handled over the same network interfaces.

#### Network Considerations for GPU Clients

- **Bandwidth Needs**: The network link between the GPU clients and the AIStore cluster is another critical factor. In line with the general rule of 'the more, the merrier,' having more bandwidth in this link is always advantageous.

#### Data Retrieval and Transmission Mechanics

- **Data Retrieval in AIStore**: Unlike systems like HDFS where objects are striped across multiple nodes, AIStore handles each object GET request from a single target node. This means no client-side re-assembly is required since the data is streamed through a single HTTP socket.

- **Handling Multiple Parallel Requests**: A target node can manage multiple GET or PUT requests concurrently. Similarly, a GPU client node can initiate several parallel requests. For example, with 8 GPUs per node and PyTorch running 5 dataloaders per GPU, there could be up to 40 shards being streamed simultaneously per client node.

- **Cluster-Wide Data Distribution**: With objects distributed across different nodes in the cluster, multiple target nodes can simultaneously respond to a single client node. This setup enables the efficient use of network bandwidth, as several nodes can saturate the network link of a client node at any given time.

### Kubernetes CNI Plugin

When setting up AIStore in a Kubernetes environment, selecting an appropriate Container Network Interface (CNI) plugin is important for ensuring high performance. Here's what to consider:

- **Performance is Key**: Opt for a high-performance CNI plugin. The primary goal is to ensure efficient network communication within your Kubernetes cluster.

- **Single Interface Configuration**: Currently, we configure only one network interface per Pod. This means there's no specific need for advanced networking features like a separate cluster-private network for rebalancing or control traffic. As a result, complex setups like Multus are not required for AIStore deployments.

- **Experiments with Different Plugins**: We have successfully tested AIStore with both Calico and Flannel CNI plugins. Each of these has shown seamless integration and performance.

- **Plugin Preference Based on Deployment Type**:
  
  - **Managed Kubernetes**: In managed Kubernetes environments, Flannel is often the preferred choice due to its simplicity and ease of integration.
  
  - **Bare-Metal Deployments**: For deployments on bare-metal infrastructure, Calico is favored for its robust networking capabilities and performance efficiency.

### Managing In-Cluster vs. External Clients in AIStore

The choice between using in-cluster clients and external clients for AIStore has implications for ease of management and setup. Here's a detailed look at both approaches:

#### In-Cluster Clients: Simplified Management

- **Ease of Management**: In-cluster clients offer a more straightforward management experience. Their integration within the Kubernetes environment streamlines various processes. Daemons, including proxies and targets, are easily accessible via the `servicePort` specified in their configuration, facilitating inter-service communication.

- **Utilizing Kubernetes DNS**: These clients can easily access AIStore endpoints using Kubernetes DNS names. This is particularly useful for connecting to the clusterIP proxy service, simplifying network configurations.

- **No Need for Complex Configurations**: With in-cluster clients, there's no requirement to set up ingress or other complex network configurations, as everything is managed within the Kubernetes ecosystem.

#### External Clients: Additional Setup Required

- **Ingress Setup**: For external clients to access the AIStore cluster, you will need to establish ingress. This involves additional configuration steps not required for in-cluster clients. In the [deployment guide](README.md) we use `hostPort` to map a container's port to a corresponding port on the host machine to facilitate external access. 

- **Port Configuration**: It's necessary to open specific ports for the targets and proxies to ensure external clients can connect. The necessary port information is detailed in the [deployment guide](README.md).

- **Performance Considerations**: Despite the differences in setup and management, the performance for in-cluster and external clients remains consistent. Both client types can achieve similar levels of efficiency and speed in data handling.

> Note: For deploying multiple targets on a single host machine, please refer our [documentation](multiple_targets_per_node.md).

#### LoadBalancer and Ingress

When using external clients, it's recommended to have a load balancer in place. This ensures clients can connect to a single, well-known IP address or DNS entry. To setup a load balancer you will need **external IPs**. The number of external IPs needed equals the number of targets plus one for the proxy.

**Setting up external IPs**
- **Bare-Metal On-Premises Deployments**: For these setups, we recommend using [MetalLB](https://metallb.universe.tf/), a popular solution for on-premises Kubernetes environments.
- **Cloud-Based Deployments**: If your AIStore is running in a cloud environment, you can utilize standard HTTP load balancer services provided by the cloud provider.

- **Proxy and Target Load Balancers**:
   - **Proxy LB**: A single load balancer consolidates proxy access, creating a high-availability endpoint for the clusterIP service.
   - **Target LBs**: Individual load balancers for each target direct traffic to specific AIStore targets, facilitating ingress rather than distributing load.

**Automating Load Balancer Setup**:
You can manually configure your load balancers or enable automatic setup by setting `externalLB` to `true` in your AIStore Custom Resource specification, allowing the AIS Operator to handle the configuration on your behalf.

### Host Performance Tuning

To efficiently handle high HTTP GET/PUT loads in AIStore, several tuning parameters are necessary, focusing on socket counts, port numbers, and port reuse. These are detailed in the provided [playbooks](../playbooks). For setups with high-performance expectations or high bandwidth networking, additional tuning, especially for HDDs involving I/O scheduler adjustments, is recommended and also outlined in the playbooks. For further guidance on enhancing AIStore's performance, refer to the supplementary [document](https://aiatscale.org/docs/performance).
