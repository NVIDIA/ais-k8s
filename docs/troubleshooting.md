## Split-brain Clusters

If there is a series of sustained network disconnections, it is possible for some subset of AIS nodes in a cluster to eventually determine that the primary proxy is unreachable and re-elect a new one. 
If this happens and the original primary proxy is in fact still running, the result will be two clusters that are unable to communicate properly, a scenario we've termed "split-brain".

### Primary proxies

As of writing, an AIS node on startup will follow this priority sequence when determining the primary proxy. See [ais/earlystart.go](https://github.com/NVIDIA/aistore/blob/main/ais/earlystart.go) for the latest.

1. `AIS_PRIMARY_EP` Environment variable (not set in k8s)
2. Cluster map
3. Config (Primary default is set to `ais-proxy-0` when the init container creates the config)

### Solving a Split-brain

Below is one reliable series of steps to solve this scenario, assuming you are using the [local-path stateStorageClass option](./state_storage.md). Other state storage options may store the metadata elsewhere, such as `/etc/ais`. 

1. Identify a working AIS node of the **same type** as the one to fix (`target` or `proxy`)
1. (optional) For proxies, remove the label from the node to avoid load balancer using it during the process.
    1. Remove the the `nvidia.com/ais-proxy=ais` or `nvidia.com/ais-target=ais` label from the broken K8s node.
    1. Delete the faulty pod. At this point it should enter `Pending` state assuming you have no other nodes available for scheduling. 
1. Use SCP to copy the working node's config from `/opt/local-path-provisioner/<pvc-name>/.ais.conf` to your machine (may require copying to your home dir and using sudo chmod)
1. Identify the node the broken proxy is scheduled on. One way is to use `kubectl get pvc -n ais <pvc-name> -o yaml` with the node's state pvc. 
1. Delete the config and cluster map from the state config path of the split node. `/opt/local-path-provisioner/<pvc-name>/.ais.conf and .ais.smap`
1. Use SCP to copy the conf to the broken node
1. Copy the conf to `/opt/local-path-provisioner/<pvc-name>/.ais.conf`
1. Label the node again to enable rescheduling: `kubectl label nodes <fixed-node> nvidia.com/ais-proxy=ais` (or nvidia.com/ais-target=ais)
1. Check the k8s node labels, e.g. `kubectl get nodes -L nvidia.com/ais-proxy,nvidia.com/ais-target`
1. Use `ais show cluster` (optionally, `ais show cluster smap --json`) to verify the cluster now contains the previously split node. 

With new versions of AIS and the CLI, we plan to support a `force` option for `ais cluster add-remove-nodes join`. This will allow easier resolution of the split-brain issue. 

### Avoiding Split-brain

Of course, having a reliable K8s cluster that doesn't experience extended network disconnections is ideal. If you don't have that, consider increasing the following options in the ais cluster config:

```
    keepalivetracker.proxy.name		 heartbeat
    keepalivetracker.proxy.interval		 10s
    keepalivetracker.proxy.factor		 3
    keepalivetracker.target.name		 heartbeat
    keepalivetracker.target.interval	 10s
    keepalivetracker.target.factor		 3
    keepalivetracker.retry_factor		 5
``` 