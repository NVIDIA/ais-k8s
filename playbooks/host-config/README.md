# AIS K8s Playbooks

A small set of convenient playbooks to assist in preparing K8s worker nodes to
host an AIStore deployment.
None of these are explicitly required.
We use all of them in our reference environments, but you're free to make the filesystem as you wish, tune nodes as you wish, etc., in which case either ignore these or use them as a reference.

These playbooks have been tested only on Ubuntu and RHEL hosts.

For playbooks related to cloud configuration, see [cloud](../cloud/README.md)

Each playbook is documented separately.
See the links in the first column below.

| Playbook(s)                                                    | Purpose                                                                                                                   |
|----------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| [ais_enable_multiqueue](docs/ais_enable_multiqueue.md)         | Enable MQ IO schedulers in Ubuntu releases for which MQ is not the default                                                |
| [ais_host_config_common](docs/ais_host_config_common.md)       | Tune worker nodes                                                                                                         |
| [ais_host_config_sysctl](docs/ais_host_config_sysctl.md)       | Tune worker node sysctl settings                                                                                          |
| ais_host_config_pcm                                            | Install the [Intel PCM tool](https://github.com/intel/pcm)                                                                |
| [ais_datafs_mkfs](docs/ais_datafs.md)                          | Create or recreate filesystems for AIStore                                                                                |
| [config_kubelet](docs/config_kubelet.md)                       | Enable K8s settings that must be set at a kubelet service level, e.g. unsafe sysctls                                      |
| ais_gpuhost_config                                             | Configure GPU compute nodes in the same cluster - install NVIDIA Docker 2, NVIDIA container runtime, etc.                 |
| [ais_gpu_host_config (EXPERIMENTAL)](./ais_gpuhost_config.yml) | Additional setup to use Nvidia GPUs on hosts. This playbook is experimental, so check the roles and use at your own risk. |

The `ais_host_config_common` playbook includes a tagging scheme to allow
more granular selection of tasks and to skip tasks that are likely site-specific.

The `vars` directory includes variable definitions that control the playbooks,
split into multiple files with comments explaining which playbooks they control
(and which tags will use them).

## Playbook Order

We run the host config playbooks in the following order:

1. Cluster hosts must be linux, preferably Ubuntu >=22.04 with ssh access from the ansible host.
1. If necessary, enable MQ IO scheduler with `ais_enable_multiqueue` and reboot.
1. Run `ais_host_config_common` on all nodes. Refer to the [playbook doc](docs/ais_host_config_common.md) to determine which tags to use. 
1. Run `ais_host_config_sysctl` on all nodes. Refer to the [playbook doc](docs/ais_host_config_sysctl.md) to determine which tags to use. 
1. If provisioning gpu worker nodes, run `ais_gpuhost_config`.
1. Make filesystems with [ais_datafs_mkfs](./docs/ais_datafs.md).
1. Establish K8s cluster using kubespray or other methods, e.g. kubeadm. 
1. Allow "unsafe" network sysctls and any other required kubelet settings with `config_kubelet`.

At this point your cluster should be ready to deploy the AIS resources, operator, and cluster. See the ais-deployment [section and guide](../ais-deployment/docs/ais_cluster_management.md)
