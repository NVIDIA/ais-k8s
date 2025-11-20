# AIStore Kubernetes (K8s) Node State Storage

Each of the AIS nodes running in a K8s cluster uses a small amount of local persistent storage for caching AIS internal config and state. 

In the past, this was **always** expected to mount to a directory on the host.
This was done in part because [local volumes in K8s](https://kubernetes.io/docs/concepts/storage/volumes/#local) do not support [dynamic provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning/), which allows for volumes to be created for each AIS node as the AIS statefulsets scale.
The host directory is configurable with `hostpathPrefix` in the AIS spec, which defaults to "/etc/ais". 

There are a few drawbacks to using [hostPath volumes](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath). 
First, there are additional security risks, as detailed in the prior link, because of the access to the host filesystem. 
Cleanup of the files on the host also presented some challenges, which become more pressing with the development of simpler [deployment with Helm](../helm/README.md).  
We needed to implement a k8s job to scan nodes for leftover directories to prevent config contamination on subsequent deployments.
Test runners also slowly built up data without their own cleanup job (which would be more complicated, due to multiple deployments on the same cluster, parallel runs, etc.).

This all leads to the introduction of `stateStorageClass` as the recommended alternative over `hostpathPrefix`.
`stateStorageClass` can be set to any local storage class that supports dynamic provisioning. Some options we've tested are the [Rancher Local Path Provisioner](https://github.com/rancher/local-path-provisioner) and [OpenEBS Local Storage](https://openebs.io/docs/concepts/data-engines/localstorage).

When `stateStorageClass` is set to a compatible storage class, the operator will automatically configure a _dynamic_ local volume.
This simplifies volume management in our StatefulSets, as volumes are automatically created and deleted according to the required persistent volume claims (PVCs).