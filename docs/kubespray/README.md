# Using Kubespray to Establish a k8s Cluster for AIStore

You can deploy AIStore on an existing k8s cluster, all you need is some persistent
storage. We use Kubespray to build an uncomplicated bare-metal k8s cluster - some
details follow, for reference.

## Prerequisites

We assume all intended k8s host nodes already have their OS installed, and that Ansible
is configured with suitable access to all nodes (passwordless sudo access on all nodes)
and that `/.ssh/known_hosts` has been pre-warmed or worked around.

## Building a k8s Cluster

1. Clone the Kubespray repo 
   ```console
   $ git clone https://github.com/kubernetes-sigs/kubespray.git
   ```

1. Install Kubespray requirements for the ansible controller:
   ```console
   $ cd kubespray
   $ sudo pip install -r requirements.txt
   ```

1. Copy the sample inventory as per the Kubespray README:
   ```console
   $ mkdir inventory/aiscluster
   $ cp -rf inventory/sample/* inventory/aiscluster/
   ```

1. We tweak a few `group_vars` as in [this sample diff](ais.diff). The Calico MTU is set to 8980 - it must be at least 20 bytes smaller than the physical netowrk MTU (which is over 9000
on our test equipment). None of the other tweaks are essential.

1. Complete the ansible inventory file `kubespray/inventory/aiscluster\hosts.ini` for those
   hosts you want to include in the cluster. We prefer, where possible, to include GPU
   nodes in the same k8s cluster. We use an inventory with group names as per
   the [reference inventory template](hosts.ini)

5. Run `kubespray` as follows:
   ```console
   $ cd kubespray
   $ ansible-playbook -i inventory/aiscluster/hosts.ini cluster.yml --become
   ```