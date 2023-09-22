# AIS K8s Playbooks

A small set of convenient playbooks to assist in preparing K8s worker nodes to
host an AIStore deployment. None of these are required.  We use all of them in our
reference environment, but you're free to make the filesystem as you wish, tune nodes
as you wish, etc., in which case either ignore these or use them as a reference.

These playbooks have been tested only on Ubuntu hosts.

For playbooks related to cloud configuration, see [cloud](cloud/README.md)

Each playbook is documented separately.  See the links in the first column below.

Playbook(s) | Useful when
----------- | -----------
[ais_enable_multiqueue](docs/ais_enable_multiqueue.md) | Enabling MQ IO schedulers in Ubuntu releases for which MQ is not the default
[ais_host_config_common](docs/ais_host_config_common.md) | Tuning worker nodes; adding useful packages etc
[ais_datafs_mkfs](docs/ais_datafs.md) | Creating or recreating filesystems for AIStore
[ais_host_post_kubespray](docs/ais_host_post_kubespray.md) | Using AIStore chart values that require "unsafe" sysctls; changes kubelet.env to enable them
ais_gpuhost_config | Configuring GPU compute nodes in the same cluster - install NVIDIA Docker 2, NVIDIA container runtime, etc.
[ais_cluster_management](docs/ais_cluster_management.md) | A collection of playbooks to deploy and upgrade AIS clusters on K8s. Cluster shut down and associated cleanup is also supported.
[ais_https_cert](docs/ais_https_cert.md) | Creates a self-signed certificate using cert-manager and stores it securely as a secret named `ais-tls-cert`. Used in HTTPS based AIStore deployments.

The `ais_host_config_common` playbook includes a tagging scheme to allow
more granular selection of tasks and to skip tasks that are likely site-specific.

The `vars` directory includes variable definitions that control the playbooks,
split into multiple files with comments explaining which playbooks they control
(and which tags will use them).

The [hosts-example.ini](hosts-example.ini) and [ansible-example.cfg](ansible-example.cfg) files are reference examples for constructing the actual hosts.ini and ansible.cfg files in the same path.

## Playbook Order

We run playbooks in the following order wrt other steps:

1. Hosts are installed with Ubuntu 18.04 LTS; ssh, ansible etc bootstrapped.
1. To enable MQ IO scheduler we run playbook `ais_enable_multiqueue` and reboot.
1. Next we run `ais_host_config_common` on all nodes (cpu and any gpu nodes).
1. Kubespray time - establish K8s cluster
1. If we're install gpu worker nodes, playbook `ais_gpuhost_config`
1. Since we use sysctl somaxconn in containers we need to change `kubelet.env` and playbook `ais_host_post_kubespray` does this for us.
1. Make filesystems with `ais_datafs_mkfs`
1. Deploy AIS K8s operator `ais_deploy_operator`
1. Deploy AIS cluster `ais_deploy_cluster`
