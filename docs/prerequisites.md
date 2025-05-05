## Prerequisites

For bare-metal deployments:
- **Kubespray Installation**: Follow the instructions provided in the [Kubespray repo](https://github.com/kubernetes-sigs/kubespray)

For both bare-metal and managed K8s deployments:
- **Kubernetes**: v1.27.x or later with [kubectl](https://kubernetes.io/docs/reference/kubectl/) installed and configured.
    - After setting up K8s, ensure all nodes are in the `Ready` state and required pods are `Running` from the controller node:
    ```
    $ kubectl get nodes
    $ kubectl get pods -A
    ```
- **Operating System (OS)**: Compatible with any Linux-based OS. Ubuntu >=22.04 or RHEL >=8 recommended.
- **Drives**:
  - AIStore's performance scales with the number and type of disks used.
  - We provide a [guide](../playbooks/host-config/docs/ais_datafs.md) to using the [ais_datafs_mkfs playbook](../playbooks/host-config/docs/ais_datafs.md) for disk formatting and mounting as required by AIS.
  - **Filesystem**:
    See the [AIStore docs](https://github.com/NVIDIA/aistore/blob/main/docs/getting_started.md#prerequisites) for information on system prerequisites. 
    Notably, AIS requires `extended attributes` and we recommend an `XFS filesystem`. 
  - **Recommended XFS mount options**:
      - noatime, nodiratime, logbufs=8, logbsize=256k, largeio, inode64, swalloc, allocsize=131072k, nobarrier.

### Resource Requirements

AIStore's demands for CPU and memory resources are generally low, with a few exceptions.
Some AIStore extensions, such as ETL (Extract, Transform, Load) processes and data re-sharding, are more compute-intensive. 

- **Re-sharding**: When dealing with large, sharded datasets, re-sharding processes use a controlled portion of the memory available to a target pod.
While having more memory is advantageous, the process is designed not to deplete the entire memory allocation.

- **ETL Processes**: For ETL operations, a separate ETL pod is initiated for each target to execute data transformations.
The intensity of computing resources used depends on the nature of these transformations.

- **Memory and Disk Utilization**: Adequate memory can reduce disk utilization.
A larger memory capacity allows for more objects to be stored in [Linux's Page Cache](https://www.thomas-krenn.com/en/wiki/Linux_Page_Cache_Basics).
This setup can lead to faster GET requests for these objects, as they can be served directly from memory, thereby reducing disk reads and lowering overall disk utilization. 

### Storage Type

- **Direct Attached Storage (DAS)**: Options like HDDs, SSDs, and NVMe drives are preferred for performance reasons.
Direct attached storage ensures fewer layers of transport, which is beneficial for performance, reliability, observability, and debugging.
Hence, we recommend locally attached storage when possible.

- **Networked Storage Options**: AIStore is also compatible with networked storage solutions like iSCSI volumes and NFS mounts.
While these options are functional, they may not offer the same level of performance as DAS.

- **Drive Selection**: If your total dataset size is relatively small, SSDs or NVMe drives are economically viable and offer superior speed. 
HDDs are more cost-effective for larger datasets. 
Aggregated data formats such as tar shards can be used to efficiently feed a large set of GPUs.

### Host Performance Tuning

To efficiently handle high HTTP GET/PUT loads in AIStore, several tuning parameters are necessary, focusing on socket counts, port numbers, and port reuse.
Some recommended configurations are detailed in the provided [playbooks](../playbooks).
For setups with high-performance expectations or high bandwidth networking, additional tuning, especially for HDDs involving I/O scheduler adjustments, is recommended and also outlined in the playbooks.

For further guidance on enhancing AIStore's performance, refer to the supplementary [blog post](https://aiatscale.org/docs/performance).
