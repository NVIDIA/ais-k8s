# ais_host_post_kubespray

## Purpose

Modifies `/etc/kubernetes/kubelet.env` to allow containers to apply the
`somaxconn` sysctl (normally considered unsafe), and restarts
the `kubelet` service for effect.

AIStore proxy and target pods under load receive a very high number of
socket connections from GPU client nodes. If average object size is small
then the connection rate is correspondingly higher, and it is easy to
overwhelm the socket listen queue depth.

The chart `values.yaml` includes a default `k8s.cluster.somaxconn` value of 0 which
avoids specifying this sysctl in the DaemonSet specifications. We recommend
a much higher value than the default listen backlog, and so (unless/until k8s
deems the sysctl safe) you need to tweak kubelet on AIStore nodes.

## Usage

(Assuming a non-zero value of `k8s.cluster.somaxconn` in AIStore chart `values.yaml`, otherwise this is not required):
```console
$ ansible-playbook -i hosts.ini ais_host_post_kubespray.yml -e playhosts=k8s-cluster --become
```

This should be run for all nodes that may host an AIStore pod.
Since out GPU nodes are labelled to run non-electable proxy
pods we apply it throughout the cluster.