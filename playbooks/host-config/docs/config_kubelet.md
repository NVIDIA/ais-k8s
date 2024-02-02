# config_kubelet.md

## Purpose

Replaces `kubelet-extra-args.conf` in `/etc/systemd/system/kubelet.service.d/` on each of the kuberenetes nodes and restarts the service to apply the new config.

The file provided by default allows containers to apply any sysctls in the `net` namespace. As of this writing, we use it primarily to enable `net.core.somaxconn` in our containers which is an "unsafe" syctl, i.e. not isolated between different pods on a node. See the [k8s docs on syctls](https://kubernetes.io/docs/tasks/administer-cluster/sysctl-cluster/). If you have existing options or additional extra args to add to the kubelet, modify the [conf file in the role](../roles/config_kubelet/files/ubelet-extra-args.conf)

## net.core.somaxconn

AIStore proxy and target Pods under load receive a very high number of
socket connections from GPU client nodes. 
If average object size is small then the connection rate is correspondingly higher, and it is easy to
overwhelm the socket listen queue depth.
Modifying `net.core.somaxconn` increases this queue. 

## Usage

```console
$ ansible-playbook -i ../hosts.ini config_kubelet.yml
```

This should be run for all nodes that may host an AIStore Pod.
