# AIStore Kubernetes (K8s) Node State Storage

Each of the AIS nodes running in a K8s cluster uses a small amount of local persistent storage for configuration and environment variables. 

In the past, this was expected to mount to a directory on the host.
This was done in part because [local volumes in K8s](https://kubernetes.io/docs/concepts/storage/volumes/#local) do not support [dynamic provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/), which allows for volumes to be created for each AIS node as the AIS statefulsets scale.
The host directory was configurable with **hostpathPrefix** in the AIS spec, which defaulted to "/etc/ais". 

There are a few drawbacks to using [hostPath volumes](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath). 
First, there are additional security risks, as detailed in the prior link, because of the access to the host filesystem. 
Cleanup of the files on the host also presented some challenges, which become more pressing with the development of simpler [deployment with Helm](../helm/README.md).  
We needed to implement a k8s job to scan nodes for leftover directories to prevent config contamination on subsequent deployments.
Test runners also slowly built up data without their own cleanup job (which would be more complicated, due to multiple deployments on the same cluster, parallel runs, etc.).

This all leads to the introduction of **stateStorageClass** and the deprecation of **hostpathPrefix**.
By setting **stateStorageClass** to `local-path` in an AIS deployment, the operator will automatically configure a _dynamic_ local volume with the [Rancher Local Path Provisioner](https://github.com/rancher/local-path-provisioner).
This simplifies volume management in our statefulsets, as volumes are automatically created and deleted according to the required persistent volume claims (PVCs). 

If a user wants to use a different dynamic storage class, stateStorageClass also allows for any storage class that supports dynamic provisioning, as long as it already exists on the K8s cluster. 